package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path"
	"slices"
	"syscall"

	"github.com/pdok/texel/config"
	"github.com/pdok/texel/pointindex"

	"github.com/go-spatial/geom"

	"github.com/carlmjohnson/versioninfo"

	"github.com/pdok/texel/processing"
	"github.com/pdok/texel/tms20"

	"github.com/iancoleman/strcase"
	"github.com/pdok/texel/processing/gpkg"
	"github.com/pdok/texel/snap"
	"github.com/urfave/cli/v2"
)

const (
	SOURCE              string = `sourceGpkg`
	TARGET              string = `targetGpkg`
	OVERWRITE           string = `overwrite`
	TILEMATRIXSET       string = `tilematrixset`
	TILEMATRICES        string = `tilematrices`
	PAGESIZE            string = `pagesize`
	KEEPPOINTSANDLINES  string = `keeppointsandlines`
	IGNOREOUTSIDEGRID   string = `ignoreoutsidegrid`
	REVERSEWINDINGORDER string = `reversewindingorder`
	ENCODETILES         string = `encodetiles`
	TILEBUFFER          string = `tilebuffer`
	USELINETRACE        string = `uselinetrace`

	MVTSOURCE  string = `mvtSourceGpkg`
	MVTOUTDIR  string = `mvtOutDir`
	TILEMATRIX string = `tilematrix`
)

//nolint:funlen
func main() {
	app := cli.NewApp()
	app.Name = "texel"
	app.Usage = "A Golang Polygon Snapping application"
	app.Version = versioninfo.Short()

	// `texel` without command defaults to `snap`.
	app.DefaultCommand = "snap"

	// Define two commands: `texel snap` and `texel mvt`.
	app.Commands = []*cli.Command{
		{
			Name:  "snap",
			Usage: "Snap polygons in a source GeoPackage to a tile grid and write the result to a target GeoPackage",
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:     SOURCE,
					Aliases:  []string{"s"},
					Usage:    "Source GPKG",
					Required: true,
					EnvVars:  []string{strcase.ToScreamingSnake(SOURCE)},
				},
				&cli.StringFlag{
					Name:     TARGET,
					Aliases:  []string{"t"},
					Usage:    "Target GPKG (prefix). One GPKG per tile matrix cq zoom level will be created and the filename will be suffixed. E.g. target_6.gpkg",
					Required: true,
					EnvVars:  []string{strcase.ToScreamingSnake(TARGET)},
				},
				&cli.BoolFlag{
					Name:     OVERWRITE,
					Aliases:  []string{"o"},
					Usage:    "Overwrite a target GPKG if it exists",
					Required: false,
					EnvVars:  []string{strcase.ToScreamingSnake(OVERWRITE)},
				},
				&cli.StringFlag{
					Name:     TILEMATRIXSET,
					Aliases:  []string{"tms"},
					Usage:    `ID of a (built-in) tile matrix set. E.g.: NetherlandsRDNewQuad`,
					Required: true,
					EnvVars:  []string{strcase.ToScreamingSnake(TILEMATRIXSET)},
				},
				&cli.StringFlag{
					Name:     TILEMATRICES,
					Aliases:  []string{"z"},
					Usage:    `IDs (usually the same as the zoom levels) of the tile matrices in the tile matrix set that should be processed for. JSON array of integers. E.g.: [4,5,6,7,8]`,
					Required: true,
					EnvVars:  []string{strcase.ToScreamingSnake(TILEMATRICES)},
				},
				&cli.IntFlag{
					Name:     PAGESIZE,
					Aliases:  []string{"p"},
					Usage:    "Page Size, how many features are written per transaction to a target GPKG",
					Value:    1000,
					Required: false,
					EnvVars:  []string{strcase.ToScreamingSnake(PAGESIZE)},
				},
				&cli.BoolFlag{
					Name:    KEEPPOINTSANDLINES,
					Aliases: []string{"pl"},
					Usage:   "Parts of polygons are reduced to points and lines after texel, keep these details or not.",
					// TODO: This results in broken tiles, we should change this to a collapse option (optionally collapse rings from polygons to lines and points; grow lines as long as possible and add an option for line-length-to-keep)
					Value:    false,
					Required: false,
					EnvVars:  []string{strcase.ToScreamingSnake(KEEPPOINTSANDLINES)},
				},
				&cli.BoolFlag{
					Name:     IGNOREOUTSIDEGRID,
					Aliases:  []string{"iog"},
					Usage:    "Ignore polygons that fall (partly) outside the grid, instead of panicking",
					Value:    false,
					Required: false,
					EnvVars:  []string{strcase.ToScreamingSnake(IGNOREOUTSIDEGRID)},
				},
				&cli.BoolFlag{
					Name:     REVERSEWINDINGORDER,
					Aliases:  []string{"rwo"},
					Usage:    "Reverse the winding order of rings in polygons, contrary to the OGC simple features spec",
					Value:    false,
					Required: false,
					EnvVars:  []string{strcase.ToScreamingSnake(REVERSEWINDINGORDER)},
				},
				&cli.BoolFlag{
					Name:     ENCODETILES,
					Aliases:  []string{"enc"},
					Usage:    "Add tables with mapbox-encoded geometries.",
					Value:    false,
					Required: false,
					EnvVars:  []string{strcase.ToScreamingSnake(ENCODETILES)},
				},
				&cli.UintFlag{
					Name:     TILEBUFFER,
					Aliases:  []string{"buf"},
					Usage:    "Buffer (in internal pixels) used to select the tiles a polygon's geometry touches.",
					Value:    0,
					Required: false,
					EnvVars:  []string{strcase.ToScreamingSnake(TILEBUFFER)},
				},
				&cli.BoolFlag{
					Name:     USELINETRACE,
					Aliases:  []string{"lt"},
					Usage:    "Use precise line-trace tile classification instead of a simple buffered bounding box to select tiles.",
					Value:    false,
					Required: false,
					EnvVars:  []string{strcase.ToScreamingSnake(USELINETRACE)},
				},
			},
			Action: func(c *cli.Context) error {
				tileMatrixSet, err := tms20.LoadEmbeddedTileMatrixSet(c.String(TILEMATRIXSET))
				if err != nil {
					return err
				}
				var tileMatrixIDs []int
				err = json.Unmarshal([]byte(c.String(TILEMATRICES)), &tileMatrixIDs)
				if err != nil {
					return err
				}
				if err = validateTileMatrixSet(tileMatrixSet, tileMatrixIDs); err != nil {
					return err
				}

				_, err = os.Stat(c.String(SOURCE))
				if os.IsNotExist(err) {
					log.Fatalf("error opening source GeoPackage: %s", err)
				}

				source := gpkg.SourceGeopackage{}
				source.Init(c.String(SOURCE))
				defer source.Close()

				targetPathFmt := injectSuffixIntoPath(c.String(TARGET))

				gpkgTargets := make(map[int]*gpkg.TargetGeopackage, len(tileMatrixIDs))
				overwrite := c.Bool(OVERWRITE)
				pagesize := c.Int(PAGESIZE) // TODO divide by tile matrices count
				snapConfig := snap.Config{
					KeepPointsAndLines:  c.Bool(KEEPPOINTSANDLINES),
					IgnoreOutsideGrid:   c.Bool(IGNOREOUTSIDEGRID),
					ReverseWindingOrder: c.Bool(REVERSEWINDINGORDER),
					EncodeTiles:         c.Bool(ENCODETILES),
					Buffer:              c.Uint(TILEBUFFER),
					UseLineTrace:        c.Bool(USELINETRACE),
				}
				for _, tmID := range tileMatrixIDs {
					gpkgTargets[tmID] = initGPKGTarget(targetPathFmt, tmID, overwrite, pagesize, c.Bool(ENCODETILES))
					defer gpkgTargets[tmID].Close() // yes, supposed to go here, want to close all at end of func
				}

				tables := source.GetTableInfo()
				for _, target := range gpkgTargets {
					err = target.CreateTables(tables)
					if err != nil {
						log.Fatalf("error initialization the target GeoPackage: %s", err)
					}
				}

				log.Println("=== start snapping ===")

				// need a copied map because of type difference processing.Target vs gpkg.TargetGeopackage
				targets := make(map[int]processing.Target, len(gpkgTargets))
				for tmID, target := range gpkgTargets {
					targets[tmID] = target
				}
				// Process the tables sequentially
				for _, table := range tables {
					log.Printf("  snapping %s", table.Name)
					for _, target := range gpkgTargets {
						source.Table = table
						target.Table = table
					}
					processBySnapping(source, targets, tileMatrixSet, snapConfig)
					log.Printf("  finished %s", table.Name)
				}

				log.Println("=== done snapping ===")
				return nil
			},
		},
		{
			Name:  "mvt",
			Usage: "Build MVT tiles from an already-encoded target GeoPackage (see --" + ENCODETILES + ")",
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:     MVTSOURCE,
					Aliases:  []string{"s"},
					Usage:    `GeoPackage containing the encoded ("<table>_encoded") and attribute tables`,
					Required: true,
					EnvVars:  []string{strcase.ToScreamingSnake(MVTSOURCE)},
				},
				&cli.StringFlag{
					Name:     MVTOUTDIR,
					Aliases:  []string{"o"},
					Usage:    "Directory to write the generated <Z>/<tileX>/<tileY>.mvt files to",
					Required: true,
					EnvVars:  []string{strcase.ToScreamingSnake(MVTOUTDIR)},
				},
				&cli.UintFlag{
					Name:     TILEMATRIX,
					Aliases:  []string{"z"},
					Usage:    "Target zoomlevel",
					Required: true,
					EnvVars:  []string{strcase.ToScreamingSnake(TILEMATRIX)},
				},
			},
			Action: func(c *cli.Context) error {
				return runBuildMVTTiles(c.String(MVTSOURCE), c.String(MVTOUTDIR))
			},
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}

func validateTileMatrixSet(tms tms20.TileMatrixSet, tileMatrixIDs []tms20.TMID) error {
	deepestTMID := slices.Max(tileMatrixIDs)
	stats, deviationInUnits, deviationInPixels, err := pointindex.DeviationStats(tms, deepestTMID)
	if err != nil {
		return err
	}
	if deviationInPixels >= 1 {
		log.Printf("[WARNING] (largest) deviation is larger than 1 tile pixel (%f units) on the deepest matrix (%d)\n", deviationInUnits, deepestTMID)
		log.Println(stats)
	}
	return pointindex.IsQuadTree(tms)
}

func initGPKGTarget(targetPathFmt string, tmID int, overwrite bool, pagesize int, encodeTiles bool) *gpkg.TargetGeopackage {
	targetPath := fmt.Sprintf(targetPathFmt, tmID)
	if overwrite {
		err := os.Remove(targetPath)
		if err != nil {
			if pathError, ok := errors.AsType[*os.PathError](err); !ok || !errors.Is(pathError.Err, syscall.ENOENT) {
				log.Fatalf("could not remove target file: %e", err)
			}
		}
	}
	target := gpkg.TargetGeopackage{}
	target.Init(targetPath, pagesize)
	target.EncodeTiles = encodeTiles
	return &target
}

func injectSuffixIntoPath(p string) string {
	dir, file := path.Split(p)
	ext := path.Ext(file)
	name := file[:len(file)-len(ext)]
	return path.Join(dir, name+"_%v"+ext)
}

func processBySnapping(source processing.Source, targets map[tms20.TMID]processing.Target, tileMatrixSet tms20.TileMatrixSet, snapConfig snap.Config) {
	processing.ProcessFeatures(source, targets, func(p geom.Polygon, tmIDs []tms20.TMID) map[tms20.TMID]processing.SnapResult {
		return snap.SnapPolygon(p, tileMatrixSet, tmIDs, snapConfig)
	}, snapConfig.EncodeTiles, snapConfig.Buffer)
}

// Construct Layer info from config; init gpkg sources
// Make sure to reuse gpkg handles
// Also return helper that closes all sources
func buildLayers(z uint, rawConfig config.TomlConfig) ([]processing.Layer, func()) {
	type initedSource struct {
		source gpkg.MVTSourceGeopackage
		tables map[string]gpkg.Table
	}
	dataSourceDictionary := processing.DatasourceToDictionary(rawConfig.DataSource)
	layers := make([]processing.Layer, 0)
	sources := make(map[string]initedSource)
	for _, tileset := range rawConfig.Tileset {
		if z < tileset.MinZoom || z > tileset.MaxZoom {
			continue
		}
		for _, rawLayer := range tileset.Layer {
			if z < rawLayer.MinZoom || z > rawLayer.MaxZoom {
				continue
			}
			// Only init gpkg sources once
			if _, present := sources[rawLayer.DataSource]; !present {
				path := dataSourceDictionary[rawLayer.DataSource]
				source := gpkg.MVTSourceGeopackage{}
				source.Init(path)
				tables := source.GetTableInfo()
				tableMap := make(map[string]gpkg.Table, len(tables))
				for _, table := range tables {
					tableMap[table.Name] = table
				}
				sources[rawLayer.DataSource] = initedSource{source, tableMap}
			}
			initedSource, present := sources[rawLayer.DataSource]
			if !present {
				err := fmt.Errorf("layer %s requires datasource %s; not found", rawLayer.Name, rawLayer.DataSource)
				panic(err)
			}
			table, present := initedSource.tables[rawLayer.TableName]
			if !present {
				err := fmt.Errorf("layer %s requires table %s in datasource %s; not found", rawLayer.Name, rawLayer.TableName, rawLayer.DataSource)
				panic(err)
			}
			// Create source with this table
			source := initedSource.source
			source.Table = table
			layer := processing.BuildLayer(rawLayer.Name, source)
			layers = append(layers, layer)
		}
	}
	closeSources := func() {
		for _, s := range sources {
			s.source.Close()
		}
	}
	return layers, closeSources
}

// Initialize resources for creating vecotrtiles and delegate to processing.
func runBuildMVTTiles(sourcePath, outDir string) error {
	if _, err := os.Stat(sourcePath); os.IsNotExist(err) {
		return fmt.Errorf("source GeoPackage does not exist: %s", sourcePath)
	}

	source := gpkg.MVTSourceGeopackage{}
	source.Init(sourcePath)
	defer source.Close()

	//	mvtTarget := gpkg.MVTFileTarget{OutDir: outDir}

	tables := source.GetTableInfo()
	for _, table := range tables {
		source.Table = table
		log.Printf("  building MVT tiles for %s", table.Name)
	}
	return nil
}
