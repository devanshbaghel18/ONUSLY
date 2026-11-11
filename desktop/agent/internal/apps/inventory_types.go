package apps

import "context"

// InventoryScanner defines the interface for discovering installed desktop applications.
type InventoryScanner interface {
	ScanApplications(ctx context.Context) ([]ApplicationIdentity, error)
}

