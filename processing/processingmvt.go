package processing

// Orchestrating functionality for the mvt command

import (
	"fmt"

	"github.com/pdok/texel/config"
	"github.com/pdok/texel/tile"
)

// Orchestrator for the tile creation pipeline
func BuildAndWriteMVTTiles(source MVTSource, target MVTTarget, tableName string) error {
	// Build key index (columns) just once.
	keyIndex := tile.BuildKeyDictionary(source.AttributeColumnNames())

	tiles, err := source.ListTiles()
	if err != nil {
		return err
	}

	for _, tc := range tiles {
		encFeatRows, err := source.GetFeaturesForTile(tc.X, tc.Y, tc.Z)
		if err != nil {
			return fmt.Errorf("getting features for tile (%d,%d): %w", tc.X, tc.Y, err)
		}
		featIDs := make([]int64, len(encFeatRows))
		for i, encFeatRow := range encFeatRows {
			featIDs[i] = encFeatRow.FeatureID
		}

		attributes, err := source.GetAttributesForFeatures(featIDs)
		if err != nil {
			return fmt.Errorf("getting attributes for tile (%d,%d): %w", tc.X, tc.Y, err)
		}

		data, err := tile.BuildMVTTile(tableName, keyIndex, encFeatRows, attributes)
		if err != nil {
			return fmt.Errorf("building tile (%d,%d): %w", tc.X, tc.Y, err)
		}

		if err := target.WriteTile(tc.X, tc.Y, tc.Z, data); err != nil {
			return fmt.Errorf("writing tile (%d,%d): %w", tc.X, tc.Y, err)
		}
	}
	return nil
}

type Layer struct {
	Source MVTSource
	Name   string
}

type MvtConfig struct {
	Layers    []Layer
	Zoomlevel uint
}

func DatasourceToDictionary(sources []config.DataSource) map[string]string {
	dataSourceDictionary := make(map[string]string, 0)
	for _, dataSource := range sources {
		dataSourceDictionary[dataSource.Name] = dataSource.Path
	}
	return dataSourceDictionary
}
