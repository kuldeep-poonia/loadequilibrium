package dynamics

import (
	"math"
	"math/rand"
)

/**************** PARAMETERS ****************/

type params struct {
	rhoMax float64
	vFree  float64
	tau    float64
	c      float64
}

/**************** FIELD ****************/

type field struct {
	dt float64
	dx float64

	p params

	rho []float64
	w   []float64   // ARZ invariant: w = v + P(ρ)

	frho []float64
	fw   []float64

	n int
}

/**************** PRESSURE ****************/

func pressure(rho float64,p params)float64{
	return p.c*p.c*math.Log(1+rho/p.rhoMax)
}

func veq(rho float64,p params)float64{
	return p.vFree*(1-rho/p.rhoMax)
}

/**************** CONSTRUCTION ****************/

func newField(cells int,dt float64,p params)*field{

	return &field{
		dt:dt,
		dx:1/float64(cells),
		p:p,
		rho:make([]float64,cells),
		w:make([]float64,cells),
		frho:make([]float64,cells+1),
		fw:make([]float64,cells+1),
		n:cells,
	}
}

/**************** CFL ****************/

func (f *field) enforceCFL(){

	maxλ:=0.0

	for i:=0;i<f.n;i++{

		r:=f.rho[i]
		v:=f.w[i]-pressure(r,f.p)

		l1:=math.Abs(v)
		l2:=math.Abs(v-f.p.c)

		if l1>maxλ{maxλ=l1}
		if l2>maxλ{maxλ=l2}
	}

	if maxλ>0{

		dtMax:=f.dx/maxλ

		if f.dt>dtMax{
			f.dt=0.8*dtMax
		}
	}
}

/**************** GHOST CELLS ****************/

func (f *field) state(i int)(float64,float64){

	if i<0{
		return f.rho[0],f.w[0]
	}

	if i>=f.n{
		return f.rho[f.n-1],f.w[f.n-1]
	}

	return f.rho[i],f.w[i]
}

/**************** HLLC ****************/

func (f *field) riemann(i int){

	rl,wl:=f.state(i-1)
	rr,wr:=f.state(i)

	pl:=pressure(rl,f.p)
	pr:=pressure(rr,f.p)

	vl:=wl-pl
	vr:=wr-pr

	ql:=rl*vl
	qr:=rr*vr

	sl:=math.Min(vl-f.p.c,vr-f.p.c)
	sr:=math.Max(vl,vr)

	if sl>=0{

		f.frho[i]=ql
		f.fw[i]=ql*wl
		return
	}

	if sr<=0{

		f.frho[i]=qr
		f.fw[i]=qr*wr
		return
	}

	sm:=(qr-ql+sl*rl-sr*rr)/(sl*rl-sr*rr)

	rhoStarL:=rl*(sl-vl)/(sl-sm)
	rhoStarR:=rr*(sr-vr)/(sr-sm)

	if sm>=0{

		f.frho[i]=ql+sl*(rhoStarL-rl)
		f.fw[i]=ql*wl+sl*(rhoStarL*wl-rl*wl)

	}else{

		f.frho[i]=qr+sr*(rhoStarR-rr)
		f.fw[i]=qr*wr+sr*(rhoStarR*wr-rr*wr)
	}
}

/**************** NOISE SOURCE ****************/

func stochasticMomentum(v float64,dt float64)float64{

	return v + math.Sqrt(dt)*0.2*rand.NormFloat64()
}

/**************** STEP ****************/

func (f *field) step(in float64)float64{

	f.enforceCFL()

	for i:=0;i<=f.n;i++{
		f.riemann(i)
	}

	for i:=0;i<f.n;i++{

		f.rho[i]+=-(f.dt/f.dx)*(f.frho[i+1]-f.frho[i])

		mom:=f.rho[i]*(f.w[i]-pressure(f.rho[i],f.p))
		mom+=-(f.dt/f.dx)*(f.fw[i+1]-f.fw[i])

		if f.rho[i]>1e-6{

			v:=mom/f.rho[i]

						  v=stochasticMomentum(v,f.dt)
					    v+=f.dt*(veq(f.rho[i],f.p)-v)/f.p.tau

			// invariant bound
			        if v>f.p.vFree{v=f.p.vFree}
			          if v < -f.p.vFree{v=-f.p.vFree}

			           f.w[i]=v+pressure(f.rho[i],f.p)
		}

		if f.rho[i]<0{f.rho[i]=0}
		if f.rho[i]>f.p.rhoMax{f.rho[i]=f.p.rhoMax}
	}

	return f.frho[f.n]
}

/**************** CHARACTERISTIC RTT ****************/

func (f *field) rtt()float64{

	x:=float64(f.n-1)*f.dx
	t:=0.0

	for x>0{

		i:=int(x/f.dx)
		if i<0{i=0}
		if i>=f.n{i=f.n-1}

		v:=f.w[i]-pressure(f.rho[i],f.p)

		if v<1e-6{v=1e-6}

		dt:=f.dx/v

		t+=dt
		x-=f.dx
	}

	return t
}

/**************** USER / ENGINE (same logic) ****************/

type user struct{
	dt float64
	w float64
	n float64
	send float64
	ack float64
	a float64
	b float64
}

func (u *user) step(def float64){

	if def>0{u.w*=u.b}else{u.w+=u.a*u.dt}

	if u.n<u.w{
		u.send=(u.w-u.n)/u.dt
	}else{
		u.send=0
	}

	u.n+=(u.send-u.ack)*u.dt
	if u.n<0{u.n=0}
}

type circuit struct{
	field *field
	user *user
	hist *history
}

type history struct{
	t []float64
	r []float64
	n int
	pos int
}

func newHistory(n int)*history{
	return &history{
		t:make([]float64,n),
		r:make([]float64,n),
		n:n,
	}
}

func (h *history) push(t,r float64){
	h.t[h.pos]=t
	h.r[h.pos]=r
	h.pos=(h.pos+1)%h.n
}

func (h *history) integral(now,delay float64)float64{

	s:=0.0

	for i:=0;i<h.n-1;i++{

		if now-h.t[i]<=delay{
			dt:=h.t[i+1]-h.t[i]
			s+=0.5*(h.r[i]+h.r[i+1])*dt
		}
	}

	return s
}

type Engine struct{
	dt float64
	circuits []*circuit
	time float64
}

func (e *Engine) step(){

	for _,c:=range e.circuits{

		out:=c.field.step(c.user.send)

		delay:=c.field.rtt()

		ack:=c.hist.integral(e.time,delay)

		def:=c.user.send-out

		c.user.ack=ack
		c.user.step(def)

		c.hist.push(e.time,c.user.send)
	}

	e.time+=e.dt
}
