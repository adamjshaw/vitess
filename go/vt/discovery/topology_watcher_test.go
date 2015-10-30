package discovery

import (
	"sync"
	"testing"
	"time"

	pbt "github.com/youtube/vitess/go/vt/proto/topodata"
	"github.com/youtube/vitess/go/vt/topo"
	"github.com/youtube/vitess/go/vt/topo/test/faketopo"
	"golang.org/x/net/context"
)

func TestCellTabletsWatcher(t *testing.T) {
	checkWatcher(t, true)
}

func TestShardReplicationWatcher(t *testing.T) {
	checkWatcher(t, false)
}

func checkWatcher(t *testing.T, cellTablets bool) {
	ft := newFakeTopo()
	fhc := newFakeHealthCheck()
	t.Logf(`ft = FakeTopo(); fhc = FakeHealthCheck()`)
	var ctw *TopologyWatcher
	if cellTablets {
		ctw = NewCellTabletsWatcher(topo.Server{Impl: ft}, fhc, "aa", 10*time.Minute, 5)
	} else {
		ctw = NewShardReplicationWatcher(topo.Server{Impl: ft}, fhc, "aa", "keyspace", "shard", 10*time.Minute, 5)
	}
	t.Logf(`ctw = CellTabletsWatcher(topo.Server{ft}, fhc, "aa", 10ms, 5)`)

	// add a tablet to the topology
	ft.AddTablet("aa", 0, "host1", map[string]int32{"vt": 123})
	ctw.loadTablets()
	t.Logf(`ft.AddTablet("aa", 0, "host1", {"vt": 123}); ctw.loadTablets()`)
	want := &pbt.EndPoint{
		Uid:     0,
		Host:    "host1",
		PortMap: map[string]int32{"vt": 123},
	}
	key := EndPointToMapKey(want)
	if ep, ok := fhc.endPoints[key]; !ok || len(fhc.endPoints) != 1 {
		t.Errorf("fhc.endPoints[key] = %+v; want %+v", ep, want)
	}

	// same tablet, different port, should update (previous
	// one should go away, new one be added).
	ft.AddTablet("aa", 0, "host1", map[string]int32{"vt": 456})
	ctw.loadTablets()
	t.Logf(`ft.AddTablet("aa", 0, "host1", {"vt": 456}); ctw.loadTablets()`)
	want = &pbt.EndPoint{
		Uid:     0,
		Host:    "host1",
		PortMap: map[string]int32{"vt": 456},
	}
	key = EndPointToMapKey(want)
	if ep, ok := fhc.endPoints[key]; !ok || len(fhc.endPoints) != 1 {
		t.Errorf("fhc.endPoints[key] = %+v; want %+v", ep, want)
	}

	ctw.Stop()
}

func newFakeTopo() *fakeTopo {
	return &fakeTopo{tablets: make(map[pbt.TabletAlias]*pbt.Tablet)}
}

type fakeTopo struct {
	faketopo.FakeTopo
	mu      sync.RWMutex
	tablets map[pbt.TabletAlias]*pbt.Tablet
}

func (ft *fakeTopo) AddTablet(cell string, uid uint32, host string, ports map[string]int32) {
	ft.mu.Lock()
	defer ft.mu.Unlock()
	ta := pbt.TabletAlias{
		Cell: cell,
		Uid:  uid,
	}
	tablet := &pbt.Tablet{
		Alias:    &ta,
		Hostname: host,
		PortMap:  make(map[string]int32),
	}
	for name, port := range ports {
		tablet.PortMap[name] = port
	}
	ft.tablets[ta] = tablet
}

func (ft *fakeTopo) RemoveTablet(cell string, uid uint32) {
	ft.mu.Lock()
	defer ft.mu.Unlock()
	ta := pbt.TabletAlias{
		Cell: cell,
		Uid:  uid,
	}
	delete(ft.tablets, ta)
}

func (ft *fakeTopo) GetTabletsByCell(ctx context.Context, cell string) ([]*pbt.TabletAlias, error) {
	ft.mu.RLock()
	defer ft.mu.RUnlock()
	res := make([]*pbt.TabletAlias, 0, 1)
	for alias, tablet := range ft.tablets {
		if tablet.Alias.Cell == cell {
			res = append(res, &alias)
		}
	}
	return res, nil
}

// GetShardReplication should return all the nodes in a shard,
// but instead we cheat for this test and just return all the
// tablets in the cell.
func (ft *fakeTopo) GetShardReplication(ctx context.Context, cell, keyspace, shard string) (*topo.ShardReplicationInfo, error) {
	ft.mu.RLock()
	defer ft.mu.RUnlock()
	nodes := make([]*pbt.ShardReplication_Node, 0, 1)
	for alias, tablet := range ft.tablets {
		if tablet.Alias.Cell == cell {
			nodes = append(nodes, &pbt.ShardReplication_Node{
				TabletAlias: &alias,
			})
		}
	}
	return topo.NewShardReplicationInfo(&pbt.ShardReplication{
		Nodes: nodes,
	}, cell, keyspace, shard), nil
}

func (ft *fakeTopo) GetTablet(ctx context.Context, alias *pbt.TabletAlias) (*pbt.Tablet, int64, error) {
	ft.mu.RLock()
	defer ft.mu.RUnlock()
	return ft.tablets[*alias], 0, nil
}
