package hashutil

// Referenced from https://github.com/TomiHiltunen/geohash-golang/blob/master/geohash.go

import (
	"bytes"
	"encoding/hex"
	"strings"
)

var (
	bits      = []int{16, 8, 4, 2, 1}
	base32    = []byte("0123456789bcdefghjkmnpqrstuvwxyz")
	neighbors = [][]string{
		[]string{
			"p0r21436x8zb9dcf5h7kjnmqesgutwvy",
			"bc01fg45238967deuvhjyznpkmstqrwx",
		},
		[]string{
			"bc01fg45238967deuvhjyznpkmstqrwx",
			"p0r21436x8zb9dcf5h7kjnmqesgutwvy",
		},
		[]string{
			"14365h7k9dcfesgujnmqp0r2twvyx8zb",
			"238967debc01fg45kmstqrwxuvhjyznp",
		},
		[]string{
			"238967debc01fg45kmstqrwxuvhjyznp",
			"14365h7k9dcfesgujnmqp0r2twvyx8zb",
		},
	}
	borders = [][]string{
		[]string{
			"prxz",
			"bcfguvyz",
		},
		[]string{
			"bcfguvyz",
			"prxz",
		},
		[]string{
			"028b",
			"0145hjnp",
		},
		[]string{
			"0145hjnp",
			"028b",
		},
	}
)

// Struct for passing Box.
type BoundingBox struct {
	sw     LatLng
	ne     LatLng
	center LatLng
}

// Struct for passing LatLng values.
type LatLng struct {
	lat float64
	lng float64
}

// Create a geohash with given precision (number of characters of the resulting
// hash) based on LatLng coordinates
func EncodeWithPrecision(latitude, longitude float64, precision int) string {
	isEven := true
	lat := []float64{-90, 90}
	lng := []float64{-180, 180}
	bit := 0
	ch := 0
	var geohash bytes.Buffer
	var mid float64
	for geohash.Len() < precision {
		if isEven {
			mid = (lng[0] + lng[1]) / 2
			if longitude > mid {
				ch |= bits[bit]
				lng[0] = mid
			} else {
				lng[1] = mid
			}
		} else {
			mid = (lat[0] + lat[1]) / 2
			if latitude > mid {
				ch |= bits[bit]
				lat[0] = mid
			} else {
				lat[1] = mid
			}
		}
		isEven = !isEven
		if bit < 4 {
			bit++
		} else {
			geohash.WriteByte(base32[ch])
			bit = 0
			ch = 0
		}
	}
	return geohash.String()
}

func GetHash(latitude, longitude float64) string{
	geo_hash := EncodeWithPrecision(latitude, longitude, 4)
	geo_hash_hex := hex.EncodeToString([]byte(geo_hash))
	geo_hash_hex = strings.ToUpper(geo_hash_hex)
	return geo_hash_hex
}