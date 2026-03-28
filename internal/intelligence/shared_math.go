package intelligence

import (
	"math"
	"math/rand"
)

func sigmoid(x float64) float64 {
	return 1 / (1 + math.Exp(-x))
}

func clamp(x, lo, hi float64) float64 {
	if x < lo {
		return lo
	}
	if x > hi {
		return hi
	}
	return x
}

func clampInt(x, lo, hi int) int {
	if x < lo {
		return lo
	}
	if x > hi {
		return hi
	}
	return x
}

func dot(a, b []float64) float64 {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	s := 0.0
	for i := 0; i < n; i++ {
		s += a[i] * b[i]
	}
	return s
}

func mean(x []float64) float64 {
	if len(x) == 0 {
		return 0
	}
	s := 0.0
	for _, v := range x {
		s += v
	}
	return s / float64(len(x))
}

func avg(x []float64) float64 {
	return mean(x)
}

func clone(x []float64) []float64 {
	y := make([]float64, len(x))
	copy(y, x)
	return y
}

func vecNorm(x []float64) float64 {
	s := 0.0
	for _, v := range x {
		s += v * v
	}
	return math.Sqrt(s)
}

func norm(x []float64) float64 {
	return vecNorm(x)
}

func diff(a, b []float64) []float64 {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	d := make([]float64, n)
	for i := 0; i < n; i++ {
		d[i] = a[i] - b[i]
	}
	return d
}

func riskShape(rf []float64) [3]float64 {
	var spike, drift, m float64

	n := float64(len(rf)) + 1e-6
	for i := range rf {
		m += rf[i]
		if i > 0 {
			drift += math.Abs(rf[i] - rf[i-1])
		}
		if rf[i] > spike {
			spike = rf[i]
		}
	}

	return [3]float64{
		m / n,
		drift / n,
		spike,
	}
}

func hasNaN(x []float64) bool {
	for _, v := range x {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return true
		}
	}
	return false
}

func identityMatrix(n int, diag float64) [][]float64 {
	m := make([][]float64, n)
	for i := range m {
		m[i] = make([]float64, n)
		m[i][i] = diag
	}
	return m
}

func invertMatrix(a [][]float64) [][]float64 {
	n := len(a)
	inv := identityMatrix(n, 1)

	m := make([][]float64, n)
	for i := range m {
		m[i] = make([]float64, n)
		copy(m[i], a[i])
	}

	for i := 0; i < n; i++ {
		p := m[i][i]
		if math.Abs(p) < 1e-6 {
			p = 1e-6
		}

		for j := 0; j < n; j++ {
			m[i][j] /= p
			inv[i][j] /= p
		}

		for k := 0; k < n; k++ {
			if k == i {
				continue
			}
			f := m[k][i]
			for j := 0; j < n; j++ {
				m[k][j] -= f * m[i][j]
				inv[k][j] -= f * inv[i][j]
			}
		}
	}

	return inv
}

func randomVector(n int, scale float64) []float64 {
	v := make([]float64, n)
	for i := range v {
		v[i] = (rand.Float64()*2 - 1) * scale
	}
	return v
}

func randomMatrix(r, c int, scale float64) [][]float64 {
	m := make([][]float64, r)
	for i := range m {
		m[i] = randomVector(c, scale)
	}
	return m
}
