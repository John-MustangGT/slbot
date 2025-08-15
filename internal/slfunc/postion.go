package slfunc

import (
   "math"
   "slbot/internal/types"
)

func Distance(a, b *types.Position) float64 {
   xsq := (b.X - a.X) * (b.X - a.X)
   ysq := (b.Y - a.Y) * (b.Y - a.Y)
   zsq := (b.Z - a.Z) * (b.Z - a.Z)
   return math.Sqrt(xsq + ysq +zsq)
}

func EqualWithFuzz(a, b *types.Position, fuzz float64) bool {
   d := Distance(a, b)
   return (d < fuzz)
}

func DistanceWithoutZ(a, b *types.Position) float64 {
   copyA := &types.Position{ X: a.X, Y: a.Y, Z: 0}
   copyB := &types.Position{ X: b.X, Y: b.Y, Z: 0}
   return Distance(copyA, copyB)
}
