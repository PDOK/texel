package processing

// Orchestrating functionality for the mvt command

import (
	"fmt"

	vectorTile "github.com/go-spatial/geom/encoding/mvt/vector_tile"
	"github.com/pdok/texel/config"
	"github.com/pdok/texel/tile"
)

// Orchestrator for the tile creation pipeline
func BuildAndWriteMVTTiles(layers []Layer, zoomlevel uint, target MVTTarget) error {
	// Build key index (columns) just once.

	tiles, err := listTiles(zoomlevel, layers)
	if err != nil {
		return err
	}

	for _, coord := range tiles {
		data, err := processTile(coord, layers)
		if err != nil {
			return fmt.Errorf("building tile (%d, %d): %w", coord.X, coord.Y, err)
		}
		if err := target.WriteTile(coord.X, coord.Y, coord.Z, data); err != nil {
			return fmt.Errorf("writing tile (%d, %d): %w", coord.X, coord.Y, err)
		}
	}
	return nil
}

type Layer struct {
	Source   MVTSource
	Name     string
	keyIndex map[string]uint32
}

func BuildLayer(name string, source MVTSource) Layer {
	keydict := tile.BuildKeyDictionary(source.AttributeColumnNames())
	return Layer{
		Name:     name,
		Source:   source,
		keyIndex: keydict,
	}
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

func listTiles(zoomlevel uint, layers []Layer) ([]TileCoord, error) {
	tileSet := make(map[TileCoord]bool, 0)
	for _, layer := range layers {
		err := layer.Source.ListTiles(zoomlevel, tileSet)
		if err != nil {
			return nil, err
		}
	}

	tileList := make([]TileCoord, 0, len(tileSet))
	for tilecoord := range tileSet {
		tileList = append(tileList, tilecoord)
	}
	return tileList, nil
}

func processLayer(tileCoord TileCoord, layer Layer) (*vectorTile.Tile_Layer, error) {
	encFeatRows, err := layer.Source.GetFeaturesForTile(tileCoord.X, tileCoord.Y, tileCoord.Z)
	if err != nil {
		return nil, fmt.Errorf("getting features for tile (%d,%d): %w", tileCoord.X, tileCoord.Y, err)
	}
	featIDs := make([]int64, len(encFeatRows))
	for i, encFeatRow := range encFeatRows {
		featIDs[i] = encFeatRow.FeatureID
	}

	attributes, err := layer.Source.GetAttributesForFeatures(featIDs)
	if err != nil {
		return nil, fmt.Errorf("getting attributes for tile (%d,%d): %w", tileCoord.X, tileCoord.Y, err)
	}

	return tile.BuildLayer(layer.Name, layer.keyIndex, encFeatRows, attributes)
}

func processTile(coord TileCoord, layers []Layer) ([]byte, error) {
	encLayers := make([]*vectorTile.Tile_Layer, 0, len(layers))
	for _, layer := range layers {
		encLayer, err := processLayer(coord, layer)
		if err != nil {
			err = fmt.Errorf("during layer encoding %s: %w", layer.Name, err)
			return nil, err
		}
		encLayers = append(encLayers, encLayer)
	}
	return tile.BuildEncodeTile(encLayers)
}
