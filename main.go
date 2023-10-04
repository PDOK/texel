package main

import (
	"github.com/pdok/sieve/snap"
	"log"
	"os"

	"github.com/pdok/sieve/processing/gpkg"
	"github.com/urfave/cli/v2"
)

const SOURCE string = `source`
const TARGET string = `target`
const RESOLUTION string = `resolution`
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
		&cli.Float64Flag{
			Name:     RESOLUTION,
			Aliases:  []string{"r"},
			Usage:    "Resolution, the threshold area to determine if a feature is sieved or not",
			Value:    0.0,
			Required: false,
			EnvVars:  []string{"SIEVE_RESOLUTION"},
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

		_, err := os.Stat(c.String(SOURCE))
		if os.IsNotExist(err) {
			log.Fatalf("error opening source GeoPackage: %s", err)
		}

		source := gpkg.SourceGeopackage{}
		source.Init(c.String(SOURCE))
		defer source.Close()

		target := gpkg.TargetGeopackage{}
		target.Init(c.String(TARGET), c.Int(PAGESIZE))
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
			snap.SnapToPointCloud(source, &target)
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
