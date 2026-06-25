package dynamics

import (
	"math"
	"math/rand"
)

/**************** PARAMETERS ****************/

type Params struct {
	RhoMax float64
	VFree  float64
	Tau    float64
	C      float64
}

/**************** FIELD ****************/

type Field struct {
	Dt float64
	Dx float64

	p Params

	rho []float64
	w   []float64 // ARZ invariant: w = v + P(ρ)

	frho []float64
	fw   []float64

	n int
}

/**************** PRESSURE ****************/

func pressure(rho float64, p Params) float64 {
	return p.C * p.C * math.Log(1+rho/p.RhoMax)
}

func veq(rho float64, p Params) float64 {
	return p.VFree * (1 - rho/p.RhoMax)
}

/**************** CONSTRUCTION ****************/

func NewField(cells int, dt float64, p Params) *Field {

	return &Field{
		Dt:   dt,
		Dx:   1 / float64(cells),
		p:    p,
		rho:  make([]float64, cells),
		w:    make([]float64, cells),
		frho: make([]float64, cells+1),
		fw:   make([]float64, cells+1),
		n:    cells,
	}
}

/**************** CFL ****************/

func (f *Field) enforceCFL() {

	maxλ := 0.0

	for i := 0; i < f.n; i++ {

		r := f.rho[i]
		v := f.w[i] - pressure(r, f.p)

		l1 := math.Abs(v)
		l2 := math.Abs(v - f.p.C)

		if l1 > maxλ {
			maxλ = l1
		}
		if l2 > maxλ {
			maxλ = l2
		}
	}

	if maxλ > 0 {

		dtMax := f.Dx / maxλ

		if f.Dt > dtMax {
			f.Dt = 0.8 * dtMax
		}
	}
}

/**************** GHOST CELLS ****************/

func (f *Field) state(i int) (float64, float64) {

	if i < 0 {
		return f.rho[0], f.w[0]
	}

	if i >= f.n {
		return f.rho[f.n-1], f.w[f.n-1]
	}

	return f.rho[i], f.w[i]
}

/**************** HLLC ****************/

func (f *Field) riemann(i int) {

	rl, wl := f.state(i - 1)
	rr, wr := f.state(i)

	pl := pressure(rl, f.p)
	pr := pressure(rr, f.p)

	vl := wl - pl
	vr := wr - pr

	ql := rl * vl
	qr := rr * vr

	sl := math.Min(vl-f.p.C, vr-f.p.C)
	sr := math.Max(vl, vr)

	if sl >= 0 {

		f.frho[i] = ql
		f.fw[i] = ql * wl
		return
	}

	if sr <= 0 {

		f.frho[i] = qr
		f.fw[i] = qr * wr
		return
	}

	sm := (qr - ql + sl*rl - sr*rr) / (sl*rl - sr*rr)

	rhoStarL := rl * (sl - vl) / (sl - sm)
	rhoStarR := rr * (sr - vr) / (sr - sm)

	if sm >= 0 {

		f.frho[i] = ql + sl*(rhoStarL-rl)
		f.fw[i] = ql*wl + sl*(rhoStarL*wl-rl*wl)

	} else {

		f.frho[i] = qr + sr*(rhoStarR-rr)
		f.fw[i] = qr*wr + sr*(rhoStarR*wr-rr*wr)
	}
}

/**************** NOISE SOURCE ****************/

func stochasticMomentum(v float64, dt float64) float64 {

	return v + math.Sqrt(dt)*0.2*rand.NormFloat64()
}

/**************** STEP ****************/

func (f *Field) Step(in float64) float64 {

	f.enforceCFL()

	for i := 0; i <= f.n; i++ {
		f.riemann(i)
	}

	for i := 0; i < f.n; i++ {

		f.rho[i] += -(f.Dt / f.Dx) * (f.frho[i+1] - f.frho[i])

		mom := f.rho[i] * (f.w[i] - pressure(f.rho[i], f.p))
		mom += -(f.Dt / f.Dx) * (f.fw[i+1] - f.fw[i])

		if f.rho[i] > 1e-6 {

			v := mom / f.rho[i]

			v = stochasticMomentum(v, f.Dt)
			v += f.Dt * (veq(f.rho[i], f.p) - v) / f.p.Tau

			// invariant bound
			if v > f.p.VFree {
				v = f.p.VFree
			}
			if v < -f.p.VFree {
				v = -f.p.VFree
			}

			f.w[i] = v + pressure(f.rho[i], f.p)
		}

		if f.rho[i] < 0 {
			f.rho[i] = 0
		}
		if f.rho[i] > f.p.RhoMax {
			f.rho[i] = f.p.RhoMax
		}
	}

	return f.frho[f.n]
}

/**************** CHARACTERISTIC RTT ****************/

func (f *Field) Rtt() float64 {

	x := float64(f.n-1) * f.Dx
	t := 0.0

	for x > 0 {

		i := int(x / f.Dx)
		if i < 0 {
			i = 0
		}
		if i >= f.n {
			i = f.n - 1
		}

		v := f.w[i] - pressure(f.rho[i], f.p)

		if v < 1e-6 {
			v = 1e-6
		}

		dt := f.Dx / v

		t += dt
		x -= f.Dx
	}

	return t
}

type User struct {
	Dt   float64
	W    float64
	N    float64
	Send float64
	Ack  float64
	A    float64
	B    float64
}

func (u *User) Step(def float64) {

	if def > 0 {
		u.W *= u.B
	} else {
		u.W += u.A * u.Dt
	}

	if u.N < u.W {
		u.Send = (u.W - u.N) / u.Dt
	} else {
		u.Send = 0
	}

	u.N += (u.Send - u.Ack) * u.Dt
	if u.N < 0 {
		u.N = 0
	}
}

type Circuit struct {
	Field *Field
	User  *User
	Hist  *History
}

type History struct {
	t   []float64
	r   []float64
	n   int
	pos int
}

func NewHistory(n int) *History {
	return &History{
		t: make([]float64, n),
		r: make([]float64, n),
		n: n,
	}
}

func (h *History) Push(t, r float64) {
	h.t[h.pos] = t
	h.r[h.pos] = r
	h.pos = (h.pos + 1) % h.n
}

func (h *History) Integral(now, delay float64) float64 {

	s := 0.0

	for i := 0; i < h.n-1; i++ {

		if now-h.t[i] <= delay {
			dt := h.t[i+1] - h.t[i]
			s += 0.5 * (h.r[i] + h.r[i+1]) * dt
		}
	}

	return s
}

type Engine struct {
	Dt       float64
	Circuits []*Circuit
	Time     float64
}

func (e *Engine) Step(macroDt float64) {
	rem := macroDt
	for rem > 0 {
		stepDt := e.Dt
		if rem < e.Dt {
			stepDt = rem
		}

		for _, c := range e.Circuits {
			c.Field.Dt = stepDt // allow field to enforce CFL
			c.Field.enforceCFL()
			if c.Field.Dt < stepDt {
				stepDt = c.Field.Dt
			}
		}

		if stepDt < 1e-6 {
			stepDt = 1e-6
		}

		for _, c := range e.Circuits {
			// Actually enforceCFL was called, stepDt is now safe
			c.Field.Dt = stepDt
			out := c.Field.Step(c.User.Send)

			delay := c.Field.Rtt()

			ack := c.Hist.Integral(e.Time, delay)

			def := c.User.Send - out

			c.User.Ack = ack
			c.User.Dt = stepDt
			c.User.Step(def)

			c.Hist.Push(e.Time, c.User.Send)
		}

		e.Time += stepDt
		rem -= stepDt
	}
}

func NewEngine(dt float64) *Engine {
	return &Engine{
		Dt:       dt,
		Circuits: make([]*Circuit, 0),
		Time:     0,
	}
}

func NewCircuit(f *Field, u *User, h *History) *Circuit {
	return &Circuit{
		Field: f,
		User:  u,
		Hist:  h,
	}
}

func NewUser(dt float64) *User {
	return &User{
		Dt: dt,
		A:  0.1,
		B:  0.9,
	}
}
