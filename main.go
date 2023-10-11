package main

import (
	"errors"
	"log"
	"os"
	"syscall"

	"github.com/pdok/sieve/processing/gpkg"
	"github.com/pdok/sieve/snap"
	"github.com/urfave/cli/v2"
	"gopkg.in/yaml.v3"
)

const SOURCE string = `source`
const TARGET string = `target`
const OVERWRITE string = `overwrite`
const TILEMATRIX string = `tilematrix`
const PAGESIZE string = `pagesize`

func main() {
	app := cli.NewApp()
	app.Name = "Snappy"
	app.Usage = "A Golang Polygon Snapping application"

	app.Flags = []cli.Flag{
		&cli.StringFlag{
			Name:     SOURCE,
			Aliases:  []string{"s"},
			Usage:    "Source GPKG",
			Required: true,
			EnvVars:  []string{"SOURCE_GPKG"},
		},
		&cli.StringFlag{
			Name:     TARGET,
			Aliases:  []string{"t"},
			Usage:    "Target GPKG",
			Required: true,
			EnvVars:  []string{"TARGET_GPKG"},
		},
		&cli.BoolFlag{
			Name:     OVERWRITE,
			Aliases:  []string{"o"},
			Usage:    "Overwrite the target GPKG if it exists",
			Required: false,
			EnvVars:  []string{"OVERWRITE"},
		},
		&cli.StringFlag{
			Name:     TILEMATRIX,
			Aliases:  []string{"m"},
			Usage:    `TileMatrix (yaml or json encoded). E.g.: {"minX": -285401.92, "maxY": 903401.92, "level": 5, "cellSize": 107.52}`,
			Required: true,
			EnvVars:  []string{"TILEMATRIX"},
		},
		&cli.IntFlag{
			Name:     PAGESIZE,
			Aliases:  []string{"p"},
			Usage:    "Page Size, how many features are written per transaction to the target GPKG",
			Value:    1000,
			Required: false,
			EnvVars:  []string{"SIEVE_PAGESIZE"},
		},
	}

	app.Action = func(c *cli.Context) error {
		var tileMatrix snap.TileMatrix
		err := yaml.Unmarshal([]byte(c.String(TILEMATRIX)), &tileMatrix)
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

		log.Println("=== start sieving ===")

		// Process the tables sequentially
		for _, table := range tables {
			log.Printf("  sieving %s", table.Name)
			source.Table = table
			target.Table = table
			snap.SnapToPointCloud(source, &target, tileMatrix)
			log.Printf("  finised %s", table.Name)
		}

		log.Println("=== done sieving ===")
		return nil
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}
