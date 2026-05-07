package registry

import (
	"fmt"

	"github.com/Melphins/msync/internal/adapter"
	"github.com/Melphins/msync/internal/adapter/alembic"
	"github.com/Melphins/msync/internal/adapter/django"
	"github.com/Melphins/msync/internal/adapter/postgres"
	"github.com/Melphins/msync/internal/adapter/rails"
)

// AdapterFactory creates adapters based on type
type AdapterFactory interface {
	Create(adapterType adapter.AdapterType, connStr, migrationDir string) (adapter.Adapter, error)
	CreateFromConfig(adapterType string, connStr, migrationDir string) (adapter.Adapter, error)
	GetSupportedAdapters() []adapter.AdapterType
	IsSupported(adapterType adapter.AdapterType) bool
}

// DefaultFactory implements AdapterFactory
type DefaultFactory struct{}

// NewDefaultFactory creates a new adapter factory
func NewDefaultFactory() AdapterFactory {
	return &DefaultFactory{}
}

// Create creates an adapter by type
func (f *DefaultFactory) Create(adapterType adapter.AdapterType, connStr, migrationDir string) (adapter.Adapter, error) {
	switch adapterType {
	case adapter.AdapterTypePostgres:
		return postgres.NewPostgresAdapter(connStr, "schema_migrations", migrationDir), nil
	case adapter.AdapterTypeAlembic:
		return alembic.NewAlembicAdapter(connStr, migrationDir), nil
	case adapter.AdapterTypeRails:
		return rails.NewRailsAdapter(connStr, migrationDir), nil
	case adapter.AdapterTypeDjango:
		return django.NewDjangoAdapter(connStr, migrationDir), nil
	default:
		return nil, fmt.Errorf("unknown adapter type: %s", adapterType)
	}
}

// CreateFromConfig creates an adapter from a string type (from config)
func (f *DefaultFactory) CreateFromConfig(adapterType string, connStr, migrationDir string) (adapter.Adapter, error) {
	// Convert string to AdapterType
	parsedType := adapter.AdapterType(adapterType)
	return f.Create(parsedType, connStr, migrationDir)
}

// GetSupportedAdapters returns all supported adapter types
func (f *DefaultFactory) GetSupportedAdapters() []adapter.AdapterType {
	return []adapter.AdapterType{
		adapter.AdapterTypePostgres,
		adapter.AdapterTypeAlembic,
		adapter.AdapterTypeRails,
		adapter.AdapterTypeDjango,
	}
}

// IsSupported checks if an adapter type is supported
func (f *DefaultFactory) IsSupported(adapterType adapter.AdapterType) bool {
	for _, t := range f.GetSupportedAdapters() {
		if t == adapterType {
			return true
		}
	}
	return false
}

// Global factory instance
var globalFactory AdapterFactory = NewDefaultFactory()

// SetGlobalFactory sets the global factory (for testing/customization)
func SetGlobalFactory(factory AdapterFactory) {
	globalFactory = factory
}

// GetGlobalFactory returns the current global factory (for testing)
func GetGlobalFactory() AdapterFactory {
	return globalFactory
}

// GetAdapter creates an adapter using the global factory
func GetAdapter(adapterType adapter.AdapterType, connStr, migrationDir string) (adapter.Adapter, error) {
	return globalFactory.Create(adapterType, connStr, migrationDir)
}

// GetAdapterFromConfig creates an adapter from config using the global factory
func GetAdapterFromConfig(adapterType string, connStr, migrationDir string) (adapter.Adapter, error) {
	return globalFactory.CreateFromConfig(adapterType, connStr, migrationDir)
}
