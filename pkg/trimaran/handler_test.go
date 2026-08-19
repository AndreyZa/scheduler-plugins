package trimaran

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	clientcache "k8s.io/client-go/tools/cache"
	st "k8s.io/kubernetes/pkg/scheduler/testing"
)

func TestHandlerCacheCleanup(t *testing.T) {
	testNode := "node-1"
	pod1 := st.MakePod().Name("Pod-1").Obj()
	pod2 := st.MakePod().Name("Pod-2").Obj()
	pod3 := st.MakePod().Name("Pod-3").Obj()
	pod4 := st.MakePod().Name("Pod-4").Obj()

	tests := []struct {
		name              string
		podInfoList       []podInfo
		podToUpdate       string
		expectedCacheSize int
		expectedCachePods []string
	}{
		{
			name: "OnUpdate doesn't add unassigned pods",
			podInfoList: []podInfo{
				{Pod: pod1},
				{Pod: pod2},
				{Pod: pod3}},
			podToUpdate:       "Pod-4",
			expectedCacheSize: 1,
			expectedCachePods: []string{"Pod-4"},
		},
		{
			name: "cleanupCache doesn't delete newly added pods",
			podInfoList: []podInfo{
				{Pod: pod1},
				{Pod: pod2},
				{Pod: pod3},
				{Timestamp: time.Now(), Pod: pod4}},
			podToUpdate:       "Pod-5",
			expectedCacheSize: 2,
			expectedCachePods: []string{"Pod-4", "Pod-5"},
		},
		{
			name: "cleanupCache deletes old pods",
			podInfoList: []podInfo{
				{Timestamp: time.Now().Add(-5 * time.Minute), Pod: pod1},
				{Timestamp: time.Now().Add(-10 * time.Second), Pod: pod2},
				{Timestamp: time.Now().Add(-5 * time.Second), Pod: pod3},
			},
			expectedCacheSize: 2,
			expectedCachePods: []string{pod2.Name, pod3.Name},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := New()
			p.ScheduledPodsCache[testNode] = append(p.ScheduledPodsCache[testNode], tt.podInfoList...)
			if tt.podToUpdate != "" {
				pod := st.MakePod().Name(tt.podToUpdate).Obj()
				pod.Spec.NodeName = testNode
				oldPod := st.MakePod().Name(tt.podToUpdate).Obj()
				p.OnUpdate(oldPod, pod)
			}
			p.cleanupCache()
			assert.NotNil(t, p.ScheduledPodsCache[testNode])
			assert.Equal(t, tt.expectedCacheSize, len(p.ScheduledPodsCache[testNode]))
			for i, v := range p.ScheduledPodsCache[testNode] {
				assert.Equal(t, tt.expectedCachePods[i], v.Pod.Name)
			}
		})
	}
}

// Регресс на панику, снятую с прод-стенда 19.08.2026: информер отдаёт в
// OnDelete надгробие DeletedFinalStateUnknown, когда пропустил событие
// удаления (релист после разрыва watch — на проде это происходило при каждом
// перезапуске apiserver'а). Голое приведение типа паниковало, а паника в
// обработчике информера роняет ВЕСЬ процесс планировщика: вместе с Trimaran
// умирал и профиль sensitivityscore, к которому этот код отношения не имеет.
func TestOnDeleteHandlesTombstone(t *testing.T) {
	nodeName := "node-1"
	pod := st.MakePod().Name("Pod-1").Node(nodeName).Obj()

	newHandler := func() *PodAssignEventHandler {
		h := New()
		h.ScheduledPodsCache[nodeName] = []podInfo{{Timestamp: time.Now(), Pod: pod}}
		return h
	}

	t.Run("надгробие с подом внутри удаляет запись из кэша", func(t *testing.T) {
		h := newHandler()
		h.OnDelete(clientcache.DeletedFinalStateUnknown{Key: nodeName + "/Pod-1", Obj: pod})
		assert.Empty(t, h.ScheduledPodsCache[nodeName],
			"под из надгробия обязан быть вычищен, иначе кэш Trimaran течёт")
	})

	t.Run("надгробие с посторонним объектом не роняет процесс", func(t *testing.T) {
		h := newHandler()
		assert.NotPanics(t, func() {
			h.OnDelete(clientcache.DeletedFinalStateUnknown{Key: "мусор", Obj: "не под"})
		})
		assert.Len(t, h.ScheduledPodsCache[nodeName], 1, "чужой объект не должен трогать кэш")
	})

	t.Run("совсем посторонний тип не роняет процесс", func(t *testing.T) {
		h := newHandler()
		assert.NotPanics(t, func() { h.OnDelete("вообще не объект") })
		assert.Len(t, h.ScheduledPodsCache[nodeName], 1)
	})
}
