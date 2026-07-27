package processing

import (
	"fmt"

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
		encFeatRows, err := source.GetFeaturesForTile(tc.X, tc.Y)
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

		if err := target.WriteTile(tc.X, tc.Y, data); err != nil {
			return fmt.Errorf("writing tile (%d,%d): %w", tc.X, tc.Y, err)
		}
	}
	return nil
}
