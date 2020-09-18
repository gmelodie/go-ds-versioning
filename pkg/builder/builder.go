package builder

import (
	"errors"
	"reflect"

	"github.com/filecoin-project/go-ds-versioning/internal/migrate"
	"github.com/filecoin-project/go-ds-versioning/internal/validate"
	versioning "github.com/filecoin-project/go-ds-versioning/pkg"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/query"
)

// Builder is an interface for constructing migrations
type Builder interface {
	Reversible(down versioning.MigrationFunc) Builder
	FilterKeys([]string) Builder
	Build() (versioning.DatastoreMigration, error)
}

type migrationBuilder struct {
	oldType      reflect.Type
	newType      reflect.Type
	upFunc       reflect.Value
	filters      []query.Filter
	isReversible bool
	downFunc     reflect.Value
}

func (mb migrationBuilder) Reversible(down versioning.MigrationFunc) Builder {
	reversibleNewType, reversibleOldType, err := validate.CheckMigration(down)
	if err != nil {
		return errorBuilder{err}
	}
	if !mb.oldType.AssignableTo(reversibleOldType) || !mb.newType.AssignableTo(reversibleNewType) {
		return errorBuilder{errors.New("reversible function does not have inverse types")}
	}
	return migrationBuilder{mb.oldType, mb.newType, mb.upFunc, mb.filters, true, reflect.ValueOf(down)}
}

func (mb migrationBuilder) FilterKeys(keys []string) Builder {
	var newFilters = mb.filters
	for _, key := range keys {
		newFilters = append(newFilters, query.FilterKeyCompare{Key: key, Op: query.NotEqual})
	}
	return migrationBuilder{mb.oldType, mb.newType, mb.upFunc, newFilters, mb.isReversible, mb.downFunc}
}

func (mb migrationBuilder) Build() (versioning.DatastoreMigration, error) {
	baseMigration := dsMigration{
		query:   query.Query{Filters: mb.filters},
		oldType: mb.oldType,
		newType: mb.newType,
		upFunc:  mb.upFunc,
	}
	if !mb.isReversible {
		return &baseMigration, nil
	}
	return &reversibleDsMigration{
		dsMigration: baseMigration,
		downFunc:    mb.downFunc,
	}, nil
}

type errorBuilder struct {
	err error
}

func (eb errorBuilder) Reversible(versioning.MigrationFunc) Builder   { return eb }
func (eb errorBuilder) FilterKeys([]string) Builder                   { return eb }
func (eb errorBuilder) Build() (versioning.DatastoreMigration, error) { return nil, eb.err }

type dsMigration struct {
	query   query.Query
	oldType reflect.Type
	newType reflect.Type
	upFunc  reflect.Value
}

func (dm *dsMigration) Up(oldDs datastore.Batching, newDS datastore.Batching) ([]datastore.Key, error) {
	return migrate.Execute(dm.query, oldDs, newDS, dm.oldType, dm.upFunc)
}

type reversibleDsMigration struct {
	dsMigration
	downFunc reflect.Value
}

func (rdm *reversibleDsMigration) Down(newDs datastore.Batching, oldDs datastore.Batching) ([]datastore.Key, error) {
	return migrate.Execute(rdm.query, newDs, oldDs, rdm.newType, rdm.downFunc)
}

// NewMigrationBuilder returns an interface that can be used to build a data base migration
func NewMigrationBuilder(up versioning.MigrationFunc) Builder {
	oldType, newType, err := validate.CheckMigration(up)
	if err != nil {
		return errorBuilder{err}
	}
	return migrationBuilder{
		oldType: oldType,
		newType: newType,
		upFunc:  reflect.ValueOf(up),
	}
}
