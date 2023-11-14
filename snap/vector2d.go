package snap

import "math"

// vector2d represents the vector between two points in 2D space
type vector2d struct {
	x float64
	y float64
}

// returns the angle of a vector in relation to the x-axis in degrees (normalised to range 0-360)
func (vec vector2d) angle() float64 {
	angleRad := math.Atan2(vec.y, vec.x)
	if angleRad < 0 {
		angleRad += (2 * math.Pi)
	}
	angleDeg := angleRad * (180 / math.Pi)
	return angleDeg
}

// dot product of vector with another vector
func (vec vector2d) dot(otherVec vector2d) float64 {
	return (vec.x * otherVec.x) + (vec.y * otherVec.y)
}

// magnitude of vector
func (vec vector2d) magnitude() float64 {
	return math.Sqrt(math.Pow(vec.x, 2) + math.Pow(vec.y, 2))
}
