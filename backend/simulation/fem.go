package fem

import (
	"encoding/json"
	"math"
	"time"

	"gonum.org/v1/gonum/mat"
	"plankroad-backend/config"
	"plankroad-backend/models"
)

type MaterialProps struct {
	ElasticModulus float64
	PoissonRatio   float64
	Density        float64
	Alpha          float64
	AllowStress    float64
}

var WoodMaterials = map[string]MaterialProps{
	"柏木":   {10000.0, 0.35, 580.0, 5.0e-6, 12.0},
	"青冈木":  {12000.0, 0.32, 780.0, 4.5e-6, 15.0},
	"松木":   {9000.0, 0.38, 500.0, 5.5e-6, 10.0},
	"栎木":   {11000.0, 0.33, 700.0, 4.8e-6, 14.0},
	"杉木":   {8500.0, 0.36, 450.0, 5.2e-6, 9.0},
}

var RockMaterials = map[string]MaterialProps{
	"石灰岩":  {55000.0, 0.22, 2700.0, 8.0e-6, 25.0},
	"花岗岩":  {60000.0, 0.25, 2650.0, 7.5e-6, 30.0},
	"片麻岩":  {50000.0, 0.23, 2750.0, 8.2e-6, 28.0},
	"大理岩":  {52000.0, 0.24, 2720.0, 7.8e-6, 22.0},
	"砂岩":   {35000.0, 0.20, 2500.0, 9.0e-6, 18.0},
	"板岩":   {40000.0, 0.18, 2600.0, 8.5e-6, 20.0},
}

type FEAModel struct {
	Nodes     []models.FEMNode
	Elements  []models.FEMElement
	NodeDOFs  int
	Boundary  []int
	LoadVec   *mat.VecDense
	StiffMat  *mat.SymDense
	Displace  *mat.VecDense
	woodProp  MaterialProps
	rockProp  MaterialProps
}

type Solver struct {
	cfg *config.FEMConfig
}

func NewSolver(cfg *config.FEMConfig) *Solver {
	return &Solver{cfg: cfg}
}

func (s *Solver) Simulate(site *models.PlankroadSite, readings []models.SensorReading) (*models.StructuralSimulation, error) {
	woodProp := getWoodProp(site.WoodType)
	rockProp := getRockProp(site.RockType)

	model := s.buildModel(site, woodProp, rockProp)
	s.applyBoundaryConditions(model)
	s.applyLoads(model, site, readings, woodProp, rockProp)
	s.solve(model)
	s.computeStresses(model)

	return s.buildResult(site, model, readings, woodProp, rockProp), nil
}

func (s *Solver) buildModel(site *models.PlankroadSite, wood, rock MaterialProps) *FEAModel {
	nodes := []models.FEMNode{}
	elements := []models.FEMElement{}

	beamCount := min(site.BeamCount, 50)
	beamLength := site.TotalLength / float64(site.BeamCount) * 3.0
	beamWidth := 0.25
	beamHeight := 0.30
	rockDepth := 2.0

	elementSize := s.cfg.ElementSize
	nx := int(math.Max(5, math.Floor(beamLength/elementSize)))
	ny := 4
	nzWood := int(math.Max(2, math.Floor(beamHeight/elementSize)))
	nzRock := int(math.Max(3, math.Floor(rockDepth/elementSize)))

	nodeID := 0
	elemID := 0

	for b := 0; b < beamCount; b += max(1, beamCount/10) {
		beamX := float64(b) * beamLength * 1.5

		for i := 0; i <= nx; i++ {
			for j := 0; j <= ny; j++ {
				for k := 0; k <= nzWood; k++ {
					x := beamX + float64(i)*beamLength/float64(nx)
					y := float64(j)*beamWidth/float64(ny) - beamWidth/2
					z := float64(k) * beamHeight / float64(nzWood)

					nodes = append(nodes, models.FEMNode{
						ID:       nodeID,
						X:        x, Y: y, Z: z,
						Material: "wood",
					})
					nodeID++
				}
			}
		}

		woodStart := nodeID - (nx+1)*(ny+1)*(nzWood+1)
		for i := 0; i < nx; i++ {
			for j := 0; j < ny; j++ {
				for k := 0; k < nzWood; k++ {
					n0 := woodStart + i*(ny+1)*(nzWood+1) + j*(nzWood+1) + k
					n1 := n0 + (nzWood + 1)
					n2 := n1 + (ny+1)*(nzWood+1)
					n3 := n2 - (nzWood + 1)
					n4 := n0 + 1
					n5 := n1 + 1
					n6 := n2 + 1
					n7 := n3 + 1

					elements = append(elements, models.FEMElement{
						ID:       elemID,
						NodeIDs:  [4]int{n0, n2, n4, n6},
						Material: "wood",
					})
					elemID++
				}
			}
		}

		rockStart := nodeID
		rockNX := int(nx * 1.5)
		for i := 0; i <= rockNX; i++ {
			for j := 0; j <= ny+2; j++ {
				for k := 0; k <= nzRock; k++ {
					x := beamX - beamLength*0.2 + float64(i)*beamLength*1.4/float64(rockNX)
					y := float64(j-1)*(beamWidth+0.4)/float64(ny+2) - (beamWidth+0.4)/2
					z := -float64(k) * rockDepth / float64(nzRock) - 0.01

					nodes = append(nodes, models.FEMNode{
						ID:       nodeID,
						X:        x, Y: y, Z: z,
						Material: "rock",
					})
					nodeID++
				}
			}
		}

		for i := 0; i < rockNX; i++ {
			for j := 0; j < ny+2; j++ {
				for k := 0; k < nzRock; k++ {
					n0 := rockStart + i*(ny+3)*(nzRock+1) + j*(nzRock+1) + k
					n2 := n0 + (ny+3)*(nzRock+1)
					n4 := n0 + 1
					n6 := n2 + 1

					elements = append(elements, models.FEMElement{
						ID:       elemID,
						NodeIDs:  [4]int{n0, n2, n4, n6},
						Material: "rock",
					})
					elemID++
				}
			}
		}
	}

	totalDOFs := len(nodes) * 3
	model := &FEAModel{
		Nodes:    nodes,
		Elements: elements,
		NodeDOFs: totalDOFs,
		StiffMat: mat.NewSymDense(totalDOFs, nil),
		LoadVec:  mat.NewVecDense(totalDOFs, nil),
		Displace: mat.NewVecDense(totalDOFs, nil),
		woodProp: wood,
		rockProp: rock,
	}

	s.assembleStiffness(model)
	return model
}

func (s *Solver) assembleStiffness(model *FEAModel) {
	for _, elem := range model.Elements {
		var prop MaterialProps
		if elem.Material == "wood" {
			prop = model.woodProp
		} else {
			prop = model.rockProp
		}

		nodes := make([]*models.FEMNode, 4)
		for i, nid := range elem.NodeIDs {
			if nid < len(model.Nodes) {
				nodes[i] = &model.Nodes[nid]
			}
		}
		if nodes[0] == nil || nodes[1] == nil || nodes[2] == nil || nodes[3] == nil {
			continue
		}

		Ke := s.tetraStiffness(nodes, prop)
		if Ke == nil {
			continue
		}

		for a := 0; a < 4; a++ {
			for b := 0; b < 4; b++ {
				rowStart := elem.NodeIDs[a] * 3
				colStart := elem.NodeIDs[b] * 3
				for i := 0; i < 3; i++ {
					for j := 0; j < 3; j++ {
						r := rowStart + i
						c := colStart + j
						if r < model.NodeDOFs && c < model.NodeDOFs {
							if r <= c {
								v := model.StiffMat.At(r, c) + Ke.At(a*3+i, b*3+j)
								model.StiffMat.SetSym(r, c, v)
							}
						}
					}
				}
			}
		}
	}
}

func (s *Solver) tetraStiffness(nodes []*models.FEMNode, prop MaterialProps) *mat.Dense {
	E := prop.ElasticModulus * 1e6
	nu := prop.PoissonRatio

	coeff := E / ((1 + nu) * (1 - 2*nu))
	C := mat.NewSymDense(6, nil)
	C.SetSym(0, 0, coeff*(1-nu))
	C.SetSym(0, 1, coeff*nu)
	C.SetSym(0, 2, coeff*nu)
	C.SetSym(1, 1, coeff*(1-nu))
	C.SetSym(1, 2, coeff*nu)
	C.SetSym(2, 2, coeff*(1-nu))
	C.SetSym(3, 3, coeff*(1-2*nu)/2)
	C.SetSym(4, 4, coeff*(1-2*nu)/2)
	C.SetSym(5, 5, coeff*(1-2*nu)/2)

	x := make([]float64, 4)
	y := make([]float64, 4)
	z := make([]float64, 4)
	for i, n := range nodes {
		x[i], y[i], z[i] = n.X, n.Y, n.Z
	}

	V := calcTetraVolume(x, y, z)
	if V < 1e-10 {
		V = 1e-6
	}

	B := mat.NewDense(6, 12, nil)
	for i := 0; i < 4; i++ {
		j, k, l := (i+1)%4, (i+2)%4, (i+3)%4
		bi := y[j]*(z[k]-z[l]) - y[k]*(z[j]-z[l]) + y[l]*(z[j]-z[k])
		ci := x[k]*(z[j]-z[l]) - x[j]*(z[k]-z[l]) + x[l]*(z[j]-z[k])
		di := x[j]*(y[k]-y[l]) - x[k]*(y[j]-y[l]) + x[l]*(y[j]-y[k])

		ic := i * 3
		B.Set(0, ic, bi)
		B.Set(1, ic+1, ci)
		B.Set(2, ic+2, di)
		B.Set(3, ic, ci)
		B.Set(3, ic+1, bi)
		B.Set(4, ic+1, di)
		B.Set(4, ic+2, ci)
		B.Set(5, ic, di)
		B.Set(5, ic+2, bi)
	}
	B.Scale(1.0/(6.0*V), B)

	BT := mat.NewDense(12, 6, nil)
	BT.Copy(B.T())

	CB := mat.NewDense(6, 12, nil)
	CB.Mul(C, B)

	Ke := mat.NewDense(12, 12, nil)
	Ke.Mul(BT, CB)
	Ke.Scale(V, Ke)

	return Ke
}

func calcTetraVolume(x, y, z []float64) float64 {
	a := [3]float64{x[1] - x[0], y[1] - y[0], z[1] - z[0]}
	b := [3]float64{x[2] - x[0], y[2] - y[0], z[2] - z[0]}
	c := [3]float64{x[3] - x[0], y[3] - y[0], z[3] - z[0]}

	cross := [3]float64{
		b[1]*c[2] - b[2]*c[1],
		b[2]*c[0] - b[0]*c[2],
		b[0]*c[1] - b[1]*c[0],
	}
	dot := a[0]*cross[0] + a[1]*cross[1] + a[2]*cross[2]
	return math.Abs(dot) / 6.0
}

func (s *Solver) applyBoundaryConditions(model *FEAModel) {
	var boundary []int
	for i, n := range model.Nodes {
		if n.Material == "rock" && n.Z < -1.8 {
			boundary = append(boundary, i*3, i*3+1, i*3+2)
		}
	}
	model.Boundary = boundary

	bcSet := make(map[int]bool)
	for _, dof := range boundary {
		bcSet[dof] = true
	}

	n := model.NodeDOFs
	freeDOFs := make([]int, 0, n)
	for i := 0; i < n; i++ {
		if !bcSet[i] {
			freeDOFs = append(freeDOFs, i)
		}
	}

	nf := len(freeDOFs)
	Kff := mat.NewSymDense(nf, nil)
	Ff := mat.NewVecDense(nf, nil)
	Uf := mat.NewVecDense(nf, nil)

	for a, i := range freeDOFs {
		Ff.SetVec(a, model.LoadVec.AtVec(i))
		for b, j := range freeDOFs {
			if a <= b {
				Kff.SetSym(a, b, model.StiffMat.At(i, j))
			}
		}
	}

	var cholesky mat.Cholesky
	if ok := cholesky.Factorize(Kff); !ok {
		simplifiedSolve(model, freeDOFs, Kff, Ff, Uf)
	} else {
		cholesky.SolveVecTo(Uf, Ff)
	}

	for a, i := range freeDOFs {
		model.Displace.SetVec(i, Uf.AtVec(a))
	}
}

func simplifiedSolve(model *FEAModel, freeDOFs []int, Kff *mat.SymDense, Ff, Uf *mat.VecDense) {
	n := len(freeDOFs)
	for i := 0; i < n; i++ {
		k := Kff.At(i, i)
		if math.Abs(k) > 1e-10 {
			Uf.SetVec(i, Ff.AtVec(i)/k)
		}
	}
}

func (s *Solver) applyLoads(model *FEAModel, site *models.PlankroadSite, readings []models.SensorReading, wood, rock MaterialProps) {
	g := 9.81
	deadLoadFactor := 1.2
	liveLoadFactor := 1.4

	for _, n := range model.Nodes {
		idx := n.ID * 3
		if idx+2 >= model.NodeDOFs {
			continue
		}

		var density float64
		if n.Material == "wood" {
			density = wood.Density
		} else {
			density = rock.Density
		}

		elemVol := s.cfg.ElementSize * s.cfg.ElementSize * s.cfg.ElementSize
		Fz := -deadLoadFactor * density * elemVol * g / 4.0
		model.LoadVec.SetVec(idx+2, model.LoadVec.AtVec(idx+2)+Fz)

		if n.Material == "wood" && n.Z >= 0.0 {
			livePressure := 3.5e3
			FzLive := -liveLoadFactor * livePressure * s.cfg.ElementSize * s.cfg.ElementSize / 4.0
			model.LoadVec.SetVec(idx+2, model.LoadVec.AtVec(idx+2)+FzLive)
		}

		if len(readings) > 0 {
			avgTemp := 0.0
			for _, r := range readings {
				avgTemp += r.Temperature
			}
			avgTemp /= float64(len(readings))
			refTemp := 15.0
			deltaT := avgTemp - refTemp

			var alpha float64
			if n.Material == "wood" {
				alpha = wood.Alpha
			} else {
				alpha = rock.Alpha
			}

			E := rock.ElasticModulus * 1e6
			if n.Material == "wood" {
				E = wood.ElasticModulus * 1e6
			}
			thermalForce := E * alpha * deltaT * elemVol / s.cfg.ElementSize

			if n.X > 0 {
				model.LoadVec.SetVec(idx, model.LoadVec.AtVec(idx)+thermalForce*0.1)
			}
		}
	}

	if len(readings) > 0 {
		for _, r := range readings {
			if r.AvgStrain > 500 {
				for _, n := range model.Nodes {
					if n.Material == "wood" {
						idx := n.ID * 3
						if idx+2 < model.NodeDOFs {
							extraLoad := -r.AvgStrain * 0.01
							model.LoadVec.SetVec(idx+2, model.LoadVec.AtVec(idx+2)+extraLoad)
						}
					}
				}
			}
		}
	}
}

func (s *Solver) solve(model *FEAModel) {
	s.applyBoundaryConditions(model)
	for i, n := range model.Nodes {
		idx := i * 3
		if idx+2 < model.NodeDOFs {
			model.Nodes[i].DisplacementX = model.Displace.AtVec(idx) * 1000
			model.Nodes[i].DisplacementY = model.Displace.AtVec(idx+1) * 1000
			model.Nodes[i].DisplacementZ = model.Displace.AtVec(idx+2) * 1000
		}
	}
}

func (s *Solver) computeStresses(model *FEAModel) {
	for i, elem := range model.Elements {
		var prop MaterialProps
		if elem.Material == "wood" {
			prop = model.woodProp
		} else {
			prop = model.rockProp
		}

		avgStrain := 0.0
		avgStress := 0.0
		for _, nid := range elem.NodeIDs {
			if nid < len(model.Nodes) {
				dx := model.Nodes[nid].DisplacementX / 1000.0
				strain := math.Abs(dx) / math.Max(s.cfg.ElementSize, 0.001)
				avgStrain += strain
				avgStress += prop.ElasticModulus * strain
			}
		}
		avgStrain /= 4.0
		avgStress /= 4.0

		model.Elements[i].Strain = avgStrain
		model.Elements[i].Stress = avgStress

		for _, nid := range elem.NodeIDs {
			if nid < len(model.Nodes) {
				model.Nodes[nid].StressXX = math.Max(model.Nodes[nid].StressXX, avgStress)
				model.Nodes[nid].VonMises = math.Max(model.Nodes[nid].VonMises, avgStress)
				model.Nodes[nid].StressYY = avgStress * prop.PoissonRatio
				model.Nodes[nid].StressZZ = avgStress * prop.PoissonRatio * 0.5
			}
		}
	}
}

func (s *Solver) buildResult(site *models.PlankroadSite, model *FEAModel, readings []models.SensorReading, wood, rock MaterialProps) *models.StructuralSimulation {
	maxWoodStress, minWoodStress := 0.0, 1e10
	maxRockStress, minRockStress := 0.0, 1e10
	maxDeflection := 0.0

	for _, n := range model.Nodes {
		def := math.Sqrt(n.DisplacementX*n.DisplacementX +
			n.DisplacementY*n.DisplacementY +
			n.DisplacementZ*n.DisplacementZ)
		if def > maxDeflection {
			maxDeflection = def
		}

		if n.Material == "wood" {
			if n.VonMises > maxWoodStress {
				maxWoodStress = n.VonMises
			}
			if n.VonMises < minWoodStress {
				minWoodStress = n.VonMises
			}
		} else {
			if n.VonMises > maxRockStress {
				maxRockStress = n.VonMises
			}
			if n.VonMises < minRockStress {
				minRockStress = n.VonMises
			}
		}
	}

	if minWoodStress == 1e10 {
		minWoodStress = 0
	}
	if minRockStress == 1e10 {
		minRockStress = 0
	}

	woodAllow := wood.AllowStress
	rockAllow := rock.AllowStress
	maxStress := math.Max(maxWoodStress/woodAllow, maxRockStress/rockAllow)
	safetyFactor := 1.0 / math.Max(maxStress, 0.01)
	if safetyFactor > 10 {
		safetyFactor = 10
	}

	elemData := map[string]interface{}{
		"total_nodes":      len(model.Nodes),
		"total_elements":   len(model.Elements),
		"wood_nodes":       countMaterialNodes(model.Nodes, "wood"),
		"rock_nodes":       countMaterialNodes(model.Nodes, "rock"),
		"node_samples":     sampleNodes(model.Nodes, 100),
		"element_samples":  sampleElements(model.Elements, 200),
	}
	elemJSON, _ := json.Marshal(elemData)

	var avgTemp float64
	if len(readings) > 0 {
		for _, r := range readings {
			avgTemp += r.Temperature
		}
		avgTemp /= float64(len(readings))
	}
	thermalLoad := wood.Alpha * wood.ElasticModulus * 1e-3 * math.Abs(avgTemp-15.0)

	return &models.StructuralSimulation{
		SiteID:             site.SiteID,
		SimTime:            time.Now(),
		WoodElasticModulus: wood.ElasticModulus,
		RockElasticModulus: rock.ElasticModulus,
		WoodPoissonRatio:   wood.PoissonRatio,
		RockPoissonRatio:   rock.PoissonRatio,
		DeadLoad:           wood.Density * 9.81 * 0.075,
		LiveLoad:           3.5,
		ThermalLoad:        round4(thermalLoad),
		MaxWoodStress:      round6(maxWoodStress),
		MinWoodStress:      round6(minWoodStress),
		MaxRockStress:      round6(maxRockStress),
		MinRockStress:      round6(minRockStress),
		MaxDeflectionMM:    round6(maxDeflection),
		SafetyFactor:       round4(safetyFactor),
		ElementData:        elemJSON,
	}
}

func getWoodProp(name string) MaterialProps {
	if p, ok := WoodMaterials[name]; ok {
		return p
	}
	return MaterialProps{10000.0, 0.35, 600.0, 5.0e-6, 12.0}
}

func getRockProp(name string) MaterialProps {
	if p, ok := RockMaterials[name]; ok {
		return p
	}
	return MaterialProps{50000.0, 0.22, 2700.0, 8.0e-6, 25.0}
}

func countMaterialNodes(nodes []models.FEMNode, mat string) int {
	count := 0
	for _, n := range nodes {
		if n.Material == mat {
			count++
		}
	}
	return count
}

func sampleNodes(nodes []models.FEMNode, max int) []models.FEMNode {
	if len(nodes) <= max {
		return nodes
	}
	step := len(nodes) / max
	sampled := make([]models.FEMNode, 0, max)
	for i := 0; i < len(nodes); i += step {
		sampled = append(sampled, nodes[i])
		if len(sampled) >= max {
			break
		}
	}
	return sampled
}

func sampleElements(elems []models.FEMElement, max int) []models.FEMElement {
	if len(elems) <= max {
		return elems
	}
	step := len(elems) / max
	sampled := make([]models.FEMElement, 0, max)
	for i := 0; i < len(elems); i += step {
		sampled = append(sampled, elems[i])
		if len(sampled) >= max {
			break
		}
	}
	return sampled
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func round4(v float64) float64 {
	return math.Round(v*10000) / 10000
}

func round6(v float64) float64 {
	return math.Round(v*1e6) / 1e6
}
