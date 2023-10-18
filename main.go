package main

import (
	"errors"
	"log"
	"os"
	"syscall"

	"github.com/creasty/defaults"
	"github.com/iancoleman/strcase"
	"github.com/pdok/texel/processing/gpkg"
	"github.com/pdok/texel/snap"
	"github.com/urfave/cli/v2"
	"gopkg.in/yaml.v3"
)

const SOURCE string = `sourceGpkg`
const TARGET string = `targetGpkg`
const OVERWRITE string = `overwrite`
const TILEMATRIX string = `tilematrix`
const PAGESIZE string = `pagesize`

//nolint:funlen
func main() {
	app := cli.NewApp()
	app.Name = "texel"
	app.Usage = "A Golang Polygon Snapping application"

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
			Usage:    "Target GPKG",
			Required: true,
			EnvVars:  []string{strcase.ToScreamingSnake(TARGET)},
		},
		&cli.BoolFlag{
			Name:     OVERWRITE,
			Aliases:  []string{"o"},
			Usage:    "Overwrite the target GPKG if it exists",
			Required: false,
			EnvVars:  []string{strcase.ToScreamingSnake(OVERWRITE)},
		},
		&cli.StringFlag{
			Name:     TILEMATRIX,
			Aliases:  []string{"m"},
			Usage:    `TileMatrix (yaml or json encoded). E.g.: {"MinX": -285401.92, "MaxY": 903401.92, "Level": 5, "CellSize": 107.52}`,
			Required: true,
			EnvVars:  []string{strcase.ToScreamingSnake(TILEMATRIX)},
		},
		&cli.IntFlag{
			Name:     PAGESIZE,
			Aliases:  []string{"p"},
			Usage:    "Page Size, how many features are written per transaction to the target GPKG",
			Value:    1000,
			Required: false,
			EnvVars:  []string{strcase.ToScreamingSnake(PAGESIZE)},
		},
	}

	app.Action = func(c *cli.Context) error {
		var tileMatrix snap.TileMatrix
		// set fields that weren't supplied to default values
		err := defaults.Set(&tileMatrix)
		if err != nil {
			return err
		}
		err = yaml.Unmarshal([]byte(c.String(TILEMATRIX)), &tileMatrix)
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

		targetPath := c.String(TARGET)
		if c.Bool(OVERWRITE) {
			err := os.Remove(targetPath)
			var pathError *os.PathError
			if err != nil {
				if !(errors.As(err, &pathError) && errors.Is(pathError.Err, syscall.ENOENT)) {
					log.Fatalf("could not remove target file: %e", err)
				}
			}
		}

		target := gpkg.TargetGeopackage{}
		target.Init(targetPath, c.Int(PAGESIZE))
		defer target.Close()

		tables := source.GetTableInfo()

		err = target.CreateTables(tables)
		if err != nil {
			log.Fatalf("error initialization the target GeoPackage: %s", err)
		}

		log.Println("=== start snapping ===")

		// Process the tables sequentially
		for _, table := range tables {
			log.Printf("  snapping %s", table.Name)
			source.Table = table
			target.Table = table
			snap.SnapToPointCloud(source, &target, tileMatrix)
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
