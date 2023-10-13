package snap

// TileMatrix contains the parameters to create a PointIndex and resembles a TileMatrix from OGC TMS
// TODO use proper and full TileMatrixSet support
type TileMatrix struct {
	MinX      float64 `yaml:"MinX"`
	MaxY      float64 `yaml:"MaxY"`
	PixelSize uint    `default:"16" yaml:"PixelSize"`
	TileSize  uint    `default:"256" yaml:"TileSize"`
	Level     uint    `yaml:"Level"`    // a.k.a. ID. determines the number of tiles
	CellSize  float64 `yaml:"CellSize"` // the cell size at that Level
}

func (tm *TileMatrix) GridSize() float64 {
	return float64(pow2(tm.Level)) * float64(tm.TileSize) * tm.CellSize
}

func (tm *TileMatrix) MinY() float64 {
	return tm.MaxY - tm.GridSize()
}

func (tm *TileMatrix) MaxX() float64 {
	return tm.MinX + tm.GridSize()
}
