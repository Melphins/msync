package registry_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/Melphins/msync/internal/adapter"
	"github.com/Melphins/msync/internal/adapter/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultFactory_Create(t *testing.T) {
	factory := registry.NewDefaultFactory()

	// Test PostgreSQL
	pg, err := factory.Create(adapter.AdapterTypePostgres, "postgres://localhost/test", "")
	require.NoError(t, err)
	assert.NotNil(t, pg)
	assert.Equal(t, adapter.AdapterTypePostgres, pg.Type())
	assert.Equal(t, "PostgreSQL", pg.Name())

	// Test Alembic
	alembic, err := factory.Create(adapter.AdapterTypeAlembic, "postgres://localhost/test", "/migrations")
	require.NoError(t, err)
	assert.NotNil(t, alembic)
	assert.Equal(t, adapter.AdapterTypeAlembic, alembic.Type())
	assert.Equal(t, "Alembic", alembic.Name())

	// Test Rails
	rails, err := factory.Create(adapter.AdapterTypeRails, "postgres://localhost/test", "/db/migrate")
	require.NoError(t, err)
	assert.NotNil(t, rails)
	assert.Equal(t, adapter.AdapterTypeRails, rails.Type())
	assert.Equal(t, "Rails", rails.Name())

	// Test Django
	django, err := factory.Create(adapter.AdapterTypeDjango, "postgres://localhost/test", "/migrations")
	require.NoError(t, err)
	assert.NotNil(t, django)
	assert.Equal(t, adapter.AdapterTypeDjango, django.Type())
	assert.Equal(t, "Django", django.Name())
}

func TestDefaultFactory_Create_UnknownType(t *testing.T) {
	factory := registry.NewDefaultFactory()

	// Unknown adapter type
	_, err := factory.Create("unknown", "", "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown adapter type")
}

func TestDefaultFactory_CreateFromConfig(t *testing.T) {
	factory := registry.NewDefaultFactory()

	// Test with string adapter types
	pg, err := factory.CreateFromConfig("postgres", "postgres://localhost/test", "")
	require.NoError(t, err)
	assert.Equal(t, adapter.AdapterTypePostgres, pg.Type())

	rails, err := factory.CreateFromConfig("rails", "postgres://localhost/test", "/db/migrate")
	require.NoError(t, err)
	assert.Equal(t, adapter.AdapterTypeRails, rails.Type())

	django, err := factory.CreateFromConfig("django", "postgres://localhost/test", "/migrations")
	require.NoError(t, err)
	assert.Equal(t, adapter.AdapterTypeDjango, django.Type())
}

func TestDefaultFactory_GetSupportedAdapters(t *testing.T) {
	factory := registry.NewDefaultFactory()
	adapters := factory.GetSupportedAdapters()

	assert.Len(t, adapters, 4)
	assert.Contains(t, adapters, adapter.AdapterTypePostgres)
	assert.Contains(t, adapters, adapter.AdapterTypeAlembic)
	assert.Contains(t, adapters, adapter.AdapterTypeRails)
	assert.Contains(t, adapters, adapter.AdapterTypeDjango)
}

func TestDefaultFactory_IsSupported(t *testing.T) {
	factory := registry.NewDefaultFactory()

	assert.True(t, factory.IsSupported(adapter.AdapterTypePostgres))
	assert.True(t, factory.IsSupported(adapter.AdapterTypeAlembic))
	assert.True(t, factory.IsSupported(adapter.AdapterTypeRails))
	assert.True(t, factory.IsSupported(adapter.AdapterTypeDjango))
	assert.False(t, factory.IsSupported("unknown"))
	assert.False(t, factory.IsSupported(adapter.AdapterTypeMySQL))
}

func TestGetAdapter(t *testing.T) {
	// Reset to default factory before tests
	registry.SetGlobalFactory(registry.NewDefaultFactory())

	pg, err := registry.GetAdapter(adapter.AdapterTypePostgres, "postgres://localhost/test", "")
	require.NoError(t, err)
	assert.NotNil(t, pg)
	assert.Equal(t, adapter.AdapterTypePostgres, pg.Type())
}

func TestGetAdapterFromConfig(t *testing.T) {
	// Reset to default factory before tests
	registry.SetGlobalFactory(registry.NewDefaultFactory())

	rails, err := registry.GetAdapterFromConfig("rails", "postgres://localhost/test", "/db/migrate")
	require.NoError(t, err)
	assert.NotNil(t, rails)
	assert.Equal(t, adapter.AdapterTypeRails, rails.Type())
}

func TestSetGlobalFactory(t *testing.T) {
	// Create a custom factory that returns a test adapter
	customFactory := &testFactory{}
	registry.SetGlobalFactory(customFactory)

	// Verify the custom factory is used
	ad, err := registry.GetAdapter(adapter.AdapterTypePostgres, "", "")
	require.NoError(t, err)
	assert.Equal(t, adapter.AdapterType("test"), ad.Type())

	// Reset to default
	registry.SetGlobalFactory(registry.NewDefaultFactory())
}

// testFactory is a test factory that returns a test adapter
type testFactory struct{}

func (f *testFactory) Create(adapterType adapter.AdapterType, connStr, migrationDir string) (adapter.Adapter, error) {
	return &testAdapter{}, nil
}

func (f *testFactory) CreateFromConfig(adapterType string, connStr, migrationDir string) (adapter.Adapter, error) {
	return &testAdapter{}, nil
}

func (f *testFactory) GetSupportedAdapters() []adapter.AdapterType {
	return []adapter.AdapterType{adapter.AdapterTypePostgres}
}

func (f *testFactory) IsSupported(adapterType adapter.AdapterType) bool {
	return adapterType == adapter.AdapterTypePostgres
}

// testAdapter is a minimal adapter implementation for testing
type testAdapter struct{}

func (a *testAdapter) Type() adapter.AdapterType {
	return "test"
}

func (a *testAdapter) Name() string {
	return "Test"
}

func (a *testAdapter) MigrationTable() string {
	return ""
}

func (a *testAdapter) Connect(ctx context.Context) (*sql.DB, error) {
	return nil, nil
}

func (a *testAdapter) CurrentVersion(ctx context.Context) (string, error) {
	return "", nil
}

func (a *testAdapter) AppliedMigrations(ctx context.Context) ([]string, error) {
	return nil, nil
}

func (a *testAdapter) LoadMigrations() error {
	return nil
}

func (a *testAdapter) GetMigrations() ([]adapter.Migration, error) {
	return nil, nil
}

func (a *testAdapter) ParseMigrationFile(filePath string) (adapter.Migration, error) {
	return adapter.Migration{}, nil
}

func (a *testAdapter) IsMigrationFile(filePath string) bool {
	return false
}

func (a *testAdapter) Apply(ctx context.Context, migration adapter.Migration) error {
	return nil
}

func (a *testAdapter) ApplyAll(ctx context.Context, from string, to string, migrations []adapter.Migration) error {
	return nil
}

func (a *testAdapter) CanApply(ctx context.Context, migration adapter.Migration) (bool, string, error) {
	return true, "", nil
}

func (a *testAdapter) CurrentSchema(ctx context.Context) (*adapter.Schema, error) {
	return nil, nil
}

func (a *testAdapter) TargetSchema(ctx context.Context) (*adapter.Schema, error) {
	return nil, nil
}

func (a *testAdapter) Diff(current, target *adapter.Schema) (*adapter.SchemaDiff, error) {
	return nil, nil
}

func (a *testAdapter) FindMigrationByVersion(version string) (adapter.Migration, error) {
	return adapter.Migration{}, nil
}
