package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path"
	"syscall"

	"github.com/carlmjohnson/versioninfo"

	"github.com/pdok/texel/processing"
	"github.com/pdok/texel/tms20"

	"github.com/iancoleman/strcase"
	"github.com/pdok/texel/processing/gpkg"
	"github.com/pdok/texel/snap"
	"github.com/urfave/cli/v2"
)

const SOURCE string = `sourceGpkg`
const TARGET string = `targetGpkg`
const OVERWRITE string = `overwrite`
const TILEMATRIXSET string = `tilematrixset`
const TILEMATRICES string = `tilematrices`
const PAGESIZE string = `pagesize`

//nolint:funlen
func main() {
	app := cli.NewApp()
	app.Name = "texel"
	app.Usage = "A Golang Polygon Snapping application"
	app.Version = versioninfo.Short()

	app.Flags = []cli.Flag{
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
	}

	app.Action = func(c *cli.Context) error {
		tileMatrixSet, err := tms20.LoadEmbeddedTileMatrixSet(c.String(TILEMATRIXSET))
		if err != nil {
			return err
		}
		var tileMatrixIDs []int
		err = json.Unmarshal([]byte(c.String(TILEMATRICES)), &tileMatrixIDs)
		if err != nil {
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
		for _, tmID := range tileMatrixIDs {
			gpkgTargets[tmID] = initGPKGTarget(targetPathFmt, tmID, overwrite, pagesize)
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
			snap.ToPointCloud(source, targets, &tileMatrixSet)
			log.Printf("  finished %s", table.Name)
		}

		log.Println("=== done snapping ===")
		return nil
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}

func initGPKGTarget(targetPathFmt string, tmID int, overwrite bool, pagesize int) *gpkg.TargetGeopackage {
	targetPath := fmt.Sprintf(targetPathFmt, tmID)
	if overwrite {
		err := os.Remove(targetPath)
		var pathError *os.PathError
		if err != nil {
			if !(errors.As(err, &pathError) && errors.Is(pathError.Err, syscall.ENOENT)) {
				log.Fatalf("could not remove target file: %e", err)
			}
		}
	}
	target := gpkg.TargetGeopackage{}
	target.Init(targetPath, pagesize)
	return &target
}

func injectSuffixIntoPath(p string) string {
	dir, file := path.Split(p)
	ext := path.Ext(file)
	name := file[:len(file)-len(ext)]
	return path.Join(dir, name+"_%v"+ext)
}
