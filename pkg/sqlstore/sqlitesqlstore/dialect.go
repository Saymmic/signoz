package sqlitesqlstore

import (
	"context"
	"reflect"

	"github.com/uptrace/bun"
)

type dialect struct {
}

func (dialect *dialect) MigrateIntToTimestamp(ctx context.Context, bun bun.IDB, table string, column string) error {
	columnType, err := dialect.GetColumnType(ctx, bun, table, column)
	if err != nil {
		return err
	}

	if columnType != "INTEGER" {
		return nil
	}

	// if the columns is integer then do this
	if _, err := bun.ExecContext(ctx, `ALTER TABLE `+table+` RENAME COLUMN `+column+` TO `+column+`_old`); err != nil {
		return err
	}

	// add new timestamp column
	if _, err := bun.
		NewAddColumn().
		Table(table).
		ColumnExpr(column + " TIMESTAMP").
		Exec(ctx); err != nil {
		return err
	}

	// copy data from old column to new column, converting from int (unix timestamp) to timestamp
	if _, err := bun.
		NewUpdate().
		Table(table).
		Set(column + " = datetime(" + column + "_old, 'unixepoch')").
		Where("1=1").
		Exec(ctx); err != nil {
		return err
	}

	// drop old column
	if _, err := bun.NewDropColumn().Table(table).Column(column + "_old").Exec(ctx); err != nil {
		return err
	}

	return nil
}

func (dialect *dialect) MigrateIntToBoolean(ctx context.Context, bun bun.IDB, table string, column string) error {
	columnType, err := dialect.GetColumnType(ctx, bun, table, column)
	if err != nil {
		return err
	}

	if columnType != "INTEGER" {
		return nil
	}

	if _, err := bun.ExecContext(ctx, `ALTER TABLE `+table+` RENAME COLUMN `+column+` TO `+column+`_old`); err != nil {
		return err
	}

	// add new boolean column
	if _, err := bun.NewAddColumn().Table(table).ColumnExpr(column + " BOOLEAN").Exec(ctx); err != nil {
		return err
	}

	// copy data from old column to new column, converting from int to boolean
	if _, err := bun.
		NewUpdate().
		Table(table).
		Set(column + " = CASE WHEN " + column + "_old = 1 THEN true ELSE false END").
		Where("1=1").
		Exec(ctx); err != nil {
		return err
	}

	// drop old column
	if _, err := bun.NewDropColumn().Table(table).Column(column + "_old").Exec(ctx); err != nil {
		return err
	}

	return nil
}

func (dialect *dialect) GetColumnType(ctx context.Context, bun bun.IDB, table string, column string) (string, error) {
	var columnType string

	err := bun.
		NewSelect().
		ColumnExpr("type").
		TableExpr("pragma_table_info(?)", table).
		Where("name = ?", column).
		Scan(ctx, &columnType)
	if err != nil {
		return "", err
	}

	return columnType, nil
}

func (dialect *dialect) ColumnExists(ctx context.Context, bun bun.IDB, table string, column string) (bool, error) {
	var count int
	err := bun.NewSelect().
		ColumnExpr("COUNT(*)").
		TableExpr("pragma_table_info(?)", table).
		Where("name = ?", column).
		Scan(ctx, &count)

	if err != nil {
		return false, err
	}

	return count > 0, nil
}

func (dialect *dialect) RenameColumn(ctx context.Context, bun bun.IDB, table string, oldColumnName string, newColumnName string) (bool, error) {
	oldColumnExists, err := dialect.ColumnExists(ctx, bun, table, oldColumnName)
	if err != nil {
		return false, err
	}

	newColumnExists, err := dialect.ColumnExists(ctx, bun, table, newColumnName)
	if err != nil {
		return false, err
	}

	if !oldColumnExists && newColumnExists {
		return true, nil
	}

	_, err = bun.
		ExecContext(ctx, "ALTER TABLE "+table+" RENAME COLUMN "+oldColumnName+" TO "+newColumnName)
	if err != nil {
		return false, err
	}
	return true, nil
}

func (dialect *dialect) TableExists(ctx context.Context, bun bun.IDB, table interface{}) (bool, error) {

	count := 0
	err := bun.
		NewSelect().
		ColumnExpr("count(*)").
		Table("sqlite_master").
		Where("type = ?", "table").
		Where("name = ?", bun.Dialect().Tables().Get(reflect.TypeOf(table)).Name).
		Scan(ctx, &count)

	if err != nil {
		return false, err
	}

	if count == 0 {
		return false, nil
	}

	return true, nil
}

func (dialect *dialect) RenameTableAndModifyModel(ctx context.Context, bun bun.IDB, oldModel interface{}, newModel interface{}, cb func(context.Context) error) error {
	exists, err := dialect.TableExists(ctx, bun, newModel)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}

	_, err = bun.
		NewCreateTable().
		IfNotExists().
		Model(newModel).
		ForeignKey(`("org_id") REFERENCES "organizations" ("id")`).
		Exec(ctx)

	if err != nil {
		return err
	}

	err = cb(ctx)
	if err != nil {
		return err
	}

	_, err = bun.
		NewDropTable().
		IfExists().
		Model(oldModel).
		Exec(ctx)
	if err != nil {
		return err
	}

	return nil
}
