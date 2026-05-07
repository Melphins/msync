package adapter_test

import (
	"testing"

	"github.com/Melphins/msync/internal/adapter"
	"github.com/stretchr/testify/assert"
)

func TestMigration_Validate(t *testing.T) {
	m := adapter.Migration{
		Version:  "0001",
		Name:     "initial_schema",
		FilePath: "/path/to/0001_initial_schema.sql",
	}

	assert.NotEmpty(t, m.Version)
	assert.NotEmpty(t, m.Name)
	assert.NotEmpty(t, m.FilePath)
}

func TestSchemaDiff_Empty(t *testing.T) {
	diff := &adapter.SchemaDiff{}

	assert.Empty(t, diff.TablesAdded)
	assert.Empty(t, diff.TablesRemoved)
	assert.Empty(t, diff.ColumnsAdded)
	assert.Empty(t, diff.ColumnsRemoved)
	assert.Empty(t, diff.ColumnsModified)
}

func TestAdapterType_IsValid(t *testing.T) {
	validTypes := []adapter.AdapterType{
		adapter.AdapterTypePostgres,
		adapter.AdapterTypeMySQL,
		adapter.AdapterTypeSQLite,
		adapter.AdapterTypeAlembic,
		adapter.AdapterTypeDjango,
		adapter.AdapterTypePrisma,
		adapter.AdapterTypeRails,
		adapter.AdapterTypeFlyway,
		adapter.AdapterTypeLiquibase,
	}

	for _, typ := range validTypes {
		assert.NotEmpty(t, string(typ), "Adapter type should have a string value")
	}
}
