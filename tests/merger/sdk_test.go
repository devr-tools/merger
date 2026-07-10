package merger_test

import (
	"context"
	"testing"

	"github.com/mergerhq/merger/pkg/merger"
)

const schemaDiff = `diff --git a/db/migrations/007_add_orders.sql b/db/migrations/007_add_orders.sql
new file mode 100644
index 0000000..5555555
--- /dev/null
+++ b/db/migrations/007_add_orders.sql
@@ -0,0 +1,1 @@
+CREATE TABLE orders (id uuid primary key);
`

func TestScanThroughSDK(t *testing.T) {
	packet, err := merger.Scan(context.Background(), merger.ScanOptions{
		Diff:  schemaDiff,
		Repo:  merger.RepoRef{Owner: "acme", Name: "orders", FullName: "acme/orders"},
		Lanes: merger.DefaultLanes(),
	})
	if err != nil {
		t.Fatalf("merger.Scan: %v", err)
	}
	if len(packet.Files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(packet.Files))
	}
	if len(packet.Mutations) == 0 {
		t.Fatalf("expected at least one mutation for a schema migration")
	}
	if packet.MergeLane == "" {
		t.Fatalf("expected a merge lane to be assigned")
	}
}

func TestDefaultLanesMatchConfigDefaults(t *testing.T) {
	lanes := merger.DefaultLanes()
	if !(lanes.GreenMax < lanes.YellowMax && lanes.YellowMax < lanes.RedMax) {
		t.Fatalf("default lanes must be strictly increasing, got %+v", lanes)
	}
}
