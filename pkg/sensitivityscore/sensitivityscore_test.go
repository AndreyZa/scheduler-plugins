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
