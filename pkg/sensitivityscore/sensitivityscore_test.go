package sensitivityscore

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"k8s.io/klog/v2"
)

func TestParseWeightsLegacyFlat(t *testing.T) {
	w, err := parseWeights([]byte(`{"llc": 1.0, "numa": 0.0, "net": 1.0, "io": 1.0}`))
	if err != nil {
		t.Fatal(err)
	}
	if w.Base != (weights{}) {
		t.Errorf("legacy format must mean base=0, got %+v", w.Base)
	}
	if w.Sens != (weights{LLC: 1, NUMA: 0, Net: 1, IO: 1}) {
		t.Errorf("legacy weights must land in sensitivity, got %+v", w.Sens)
	}
}

func TestParseWeightsBasePlusSensitivity(t *testing.T) {
	w, err := parseWeights([]byte(
		`{"base": {"io": 1.0, "net": 0.09}, "sensitivity": {"llc": 0.5}}`))
	if err != nil {
		t.Fatal(err)
	}
	if w.Base != (weights{IO: 1.0, Net: 0.09}) {
		t.Errorf("base mismatch: %+v", w.Base)
	}
	if w.Sens != (weights{LLC: 0.5}) {
		t.Errorf("sensitivity mismatch: %+v", w.Sens)
	}
}

func TestParseWeightsBaseOnly(t *testing.T) {
	// Калиброванный STAGE-вариант: цена целиком базовая, sensitivity-ключа
	// нет вовсе — β не должен молча стать легаси-единицами.
	w, err := parseWeights([]byte(`{"base": {"io": 1.0}}`))
	if err != nil {
		t.Fatal(err)
	}
	if w.Sens != (weights{}) {
		t.Errorf("absent sensitivity must be zero, got %+v", w.Sens)
	}
	if w.Base.IO != 1.0 {
		t.Errorf("base.io mismatch: %+v", w.Base)
	}
}

func TestParseWeightsMalformed(t *testing.T) {
	if _, err := parseWeights([]byte(`{"llc": "high"}`)); err == nil {
		t.Fatal("malformed file must return error, not silent zeros")
	}
}

func TestInterferenceScoreLegacyEquivalence(t *testing.T) {
	// base=0 воспроизводит формулу до изменения: 100 − Σ w·s·p / Σw·100 · 100.
	w := scoreWeights{Sens: weights{LLC: 1, NUMA: 1, Net: 1, IO: 1}}
	s := sensitivityVector{LLC: 1, NUMA: 0.5, Net: 0, IO: 1}
	p := nodePressure{LLC: 100, NUMA: 50, Net: 100, IO: 0}
	// interference = 100 + 25 + 0 + 0 = 125; max = 400 => 100 − 31.25 → 68
	if got := interferenceScore(s, p, w); got != 68 {
		t.Errorf("legacy equivalence: got %d, want 68", got)
	}
}

func TestInterferenceScoreBaseChargesInsensitiveTask(t *testing.T) {
	// Суть базовой цены: задача с s=0 по оси ВСЁ РАВНО платит за её давление
	// (эмпирика STAGE: дисковый шторм замедляет все задачи узла).
	w := scoreWeights{Base: weights{IO: 1.0}}
	insensitive := sensitivityVector{} // все оси low
	storm := nodePressure{IO: 100}
	clean := nodePressure{}
	if s, c := interferenceScore(insensitive, storm, w), interferenceScore(insensitive, clean, w); s >= c {
		t.Errorf("io-storm node must score below clean for insensitive task: storm=%d clean=%d", s, c)
	}
	// А при чисто sensitivity-весах та же задача давления «не видит» —
	// прежнее поведение, которое и не смогло оценить универсальную цену.
	wOld := scoreWeights{Sens: weights{IO: 1.0}}
	if s, c := interferenceScore(insensitive, storm, wOld), interferenceScore(insensitive, clean, wOld); s != c {
		t.Errorf("sensitivity-only weights must be blind here: storm=%d clean=%d", s, c)
	}
}

func TestInterferenceScoreZeroWeights(t *testing.T) {
	// Все веса нулевые — знаменатель 0, скор максимален (нет участвующих осей).
	got := interferenceScore(
		sensitivityVector{LLC: 1}, nodePressure{LLC: 100}, scoreWeights{})
	if got != 100 {
		t.Errorf("zero weights: got %d, want 100", got)
	}
}

// Регресс на баг, найденный сверкой лабы с прод-IaC 18.08.2026: строка
// "sensitivity weights loaded" логировалась ТОЛЬКО при смене весов, поэтому
// веса, совпавшие с defaultWeights() (легаси-формат {"llc":1,...}), не
// печатали её никогда — а preflight серии (scripts/run-series.sh в bench-репо)
// ждёт её и валил прогон на исправном планировщике.
func TestReloadWeightsLogsFirstLoadEvenWhenEqualToDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "weights.json")
	// Ровно defaultWeights(): легаси-формат, все веса чувствительности = 1.
	if err := os.WriteFile(path, []byte(`{"llc":1.0,"numa":1.0,"net":1.0,"io":1.0}`), 0o644); err != nil {
		t.Fatal(err)
	}
	orig := weightsFilePath
	weightsFilePath = path
	defer func() { weightsFilePath = orig }()

	var buf bytes.Buffer
	klog.SetOutput(&buf)
	klog.LogToStderr(false)
	defer func() { klog.LogToStderr(true) }()

	s := &SensitivityScore{weights: defaultWeights()}

	s.reloadWeights()
	klog.Flush()
	if got := strings.Count(buf.String(), "sensitivity weights loaded"); got != 1 {
		t.Fatalf("первая загрузка обязана логироваться ровно один раз, получено %d; лог: %s", got, buf.String())
	}
	if !s.weightsLogged {
		t.Error("после первой загрузки weightsLogged должен быть true")
	}

	// Повторное чтение того же файла ничего не меняет — второй строки быть не должно.
	s.reloadWeights()
	klog.Flush()
	if got := strings.Count(buf.String(), "sensitivity weights loaded"); got != 1 {
		t.Fatalf("неизменные веса не должны логироваться повторно, получено %d", got)
	}

	// А вот смена весов обязана дать вторую строку.
	if err := os.WriteFile(path, []byte(`{"base":{"io":0.23},"sensitivity":{"io":0.63}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	s.reloadWeights()
	klog.Flush()
	if got := strings.Count(buf.String(), "sensitivity weights loaded"); got != 2 {
		t.Fatalf("смена весов обязана логироваться, получено %d", got)
	}
}

func TestParseWeightDocumentVariants(t *testing.T) {
	data := []byte(`{
		"base":{"io":0.2},"sensitivity":{"io":0.8},
		"variants":{
			"ss-no-sensitivity":{"base":{"io":1.0}},
			"ss-no-base":{"sensitivity":{"io":2.0}},
			"max-pressure":{"base":{"io":1.0},"aggregation":"max"}
		}
	}`)
	root, variants, err := parseWeightDocument(data)
	if err != nil {
		t.Fatal(err)
	}
	if root.Aggregation != aggregationSum {
		t.Fatalf("root aggregation = %q, want sum", root.Aggregation)
	}
	if got := variants[variantNoSensitivity].Base.IO; got != 1.0 {
		t.Fatalf("no-S base.io = %v, want 1", got)
	}
	if got := variants[variantNoBase].Sens.IO; got != 2.0 {
		t.Fatalf("no-base sensitivity.io = %v, want 2", got)
	}
	if got := variants[variantMaxPressure].Aggregation; got != aggregationMax {
		t.Fatalf("max aggregation = %q, want max", got)
	}
}

func TestParseWeightDocumentRejectsInvalidVariant(t *testing.T) {
	for name, data := range map[string]string{
		"zero":        `{"variants":{"max-pressure":{"base":{},"sensitivity":{}}}}`,
		"aggregation": `{"variants":{"max-pressure":{"base":{"io":1},"aggregation":"median"}}`,
		"max-no-base": `{"variants":{"max-pressure":{"sensitivity":{"io":1},"aggregation":"max"}}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, _, err := parseWeightDocument([]byte(data)); err == nil {
				t.Fatalf("invalid variant must be rejected: %s", data)
			}
		})
	}
}

func TestInterferenceScoreMaxPressureDiffersFromSum(t *testing.T) {
	w := scoreWeights{Base: weights{LLC: 1, IO: 1}, Aggregation: aggregationMax}
	pod := sensitivityVector{}               // no-S: low и high не меняют решение
	llcHot := nodePressure{LLC: 100, IO: 10} // max=100, sum=110
	ioHot := nodePressure{LLC: 10, IO: 90}   // max=90, sum=100
	if got := interferenceScore(pod, llcHot, w); got >= interferenceScore(pod, ioHot, w) {
		t.Fatalf("max-pressure must avoid max-priced axis: llcHot=%d ioHot=%d", got,
			interferenceScore(pod, ioHot, w))
	}
	high := sensitivityVector{LLC: 1, IO: 1}
	if got := interferenceScore(high, llcHot, w); got != interferenceScore(pod, llcHot, w) {
		t.Fatalf("no-S must not depend on S: low=%d high=%d",
			interferenceScore(pod, llcHot, w), got)
	}
}

func TestInterferenceScoreNoBaseNeedsSensitivity(t *testing.T) {
	w := scoreWeights{Sens: weights{IO: 1}}
	storm := nodePressure{IO: 100}
	if got := interferenceScore(sensitivityVector{}, storm, w); got != 100 {
		t.Fatalf("no-base with low-S must be blind: got %d", got)
	}
	if got := interferenceScore(sensitivityVector{IO: 1}, storm, w); got != 0 {
		t.Fatalf("no-base with high-S must charge pressure: got %d", got)
	}
}

func TestReloadWeightsSelectsIndependentVariants(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "weights.json")
	data := []byte(`{"base":{"io":0.2},"variants":{
		"ss-no-sensitivity":{"base":{"io":1.0}},
		"ss-no-base":{"sensitivity":{"io":2.0}},
		"max-pressure":{"base":{"io":1.0},"aggregation":"max"}
	}}`)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	orig := weightsFilePath
	weightsFilePath = path
	defer func() { weightsFilePath = orig }()

	noS := &SensitivityScore{name: NoSensitivityName, variantKey: variantNoSensitivity,
		weights: defaultWeightsForVariant(variantNoSensitivity)}
	noBase := &SensitivityScore{name: NoBaseName, variantKey: variantNoBase,
		weights: defaultWeightsForVariant(variantNoBase)}
	maxP := &SensitivityScore{name: MaxPressureName, variantKey: variantMaxPressure,
		weights: defaultWeightsForVariant(variantMaxPressure)}
	noS.reloadWeights()
	noBase.reloadWeights()
	maxP.reloadWeights()
	if noS.weights.Base.IO != 1 || noS.weights.Sens.IO != 0 {
		t.Errorf("no-S selected wrong weights: %+v", noS.weights)
	}
	if noBase.weights.Sens.IO != 2 || noBase.weights.Base.IO != 0 {
		t.Errorf("no-base selected wrong weights: %+v", noBase.weights)
	}
	if maxP.weights.Base.IO != 1 || maxP.weights.Aggregation != aggregationMax {
		t.Errorf("max selected wrong weights: %+v", maxP.weights)
	}
}

func TestVariantNames(t *testing.T) {
	for name, want := range map[string]string{
		NoSensitivityName: NoSensitivityName,
		NoBaseName:        NoBaseName,
		MaxPressureName:   MaxPressureName,
	} {
		if got := (&SensitivityScore{name: name}).Name(); got != want {
			t.Errorf("Name() = %q, want %q", got, want)
		}
	}
	if got := (&SensitivityScore{}).Name(); got != Name {
		t.Errorf("empty legacy name = %q, want %q", got, Name)
	}
}
