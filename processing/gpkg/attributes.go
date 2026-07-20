package gpkg

import (
	"fmt"
	"strings"
	"time"
)

// Batch value for reading features from the geopackage table
const maxSQLiteBatchParams = 500

// selectByFIDsSQL builds a SELECT of all the table's columns for a batch of feature IDs.
// The fid is assumed to be the table's first column.
func (t Table) selectByFIDsSQL(batchSize int) string {
	var csql []string //nolint:prealloc
	for _, c := range t.columns {
		csql = append(csql, c.name)
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", batchSize), ",")
	fidColumn := t.columns[0].name
	return `SELECT ` + strings.Join(csql, `,`) + ` FROM "` + t.Name + `" WHERE ` + fidColumn + ` IN (` + placeholders + `)`
}

// Extract features from the map construct.
func AttributesForFeature(attributes map[int64]map[string]any, featureID int64) map[string]any {
	if attrs, ok := attributes[featureID]; ok {
		return attrs
	}
	return map[string]any{}
}

// GetAttributesForFeatures fetches the attributes (i.e. all non-geometry, non-fid columns)
// for the given feature IDs, batching the query to respect SQLite's parameter limits.
// The fid is assumed to be the table's first column.
func (source SourceGeopackage) GetAttributesForFeatures(featureIDs []int64) (map[int64]map[string]any, error) {
	attributes := make(map[int64]map[string]any, len(featureIDs))
	fidColumn := source.Table.columns[0].name

	for start := 0; start < len(featureIDs); start += maxSQLiteBatchParams {
		batch := featureIDs[start:min(start+maxSQLiteBatchParams, len(featureIDs))]

		batchAsAny := make([]any, len(batch))
		for i, fid := range batch {
			batchAsAny[i] = fid
		}

		if err := source.queryAttributesBatch(fidColumn, batchAsAny, attributes); err != nil {
			return nil, err
		}
	}

	return attributes, nil
}

func (source SourceGeopackage) getColNames() []string {
	cols := source.Table.columns
	colNames := make([]string, len(cols))
	for i, col := range cols {
		colNames[i] = col.name
	}
	return colNames
}

// Read from source.Table, store rows as attributes[fid][columnName] = value for all fids
func (source SourceGeopackage) queryAttributesBatch(fidColumn string, fids []any, attributes map[int64]map[string]any) error {
	// Read relevant rows
	rows, err := source.handle.Query(source.Table.selectByFIDsSQL(len(fids)), fids...)
	if err != nil {
		return fmt.Errorf("querying attributes for %d feature(s): %w", len(fids), err)
	}
	defer rows.Close()

	cols := source.getColNames()

	// Process rows
	for rows.Next() {
		// Read data with helper vars
		vals := make([]any, len(cols))
		valPtrs := make([]any, len(cols))
		for i := range cols {
			valPtrs[i] = &vals[i]
		}
		if err := rows.Scan(valPtrs...); err != nil {
			return fmt.Errorf("scanning attribute row: %w", err)
		}

		// Interpret data
		row := make(map[string]any, len(cols))
		var fid int64
		var fidFound bool
		for i, colName := range cols {
			switch colName {
			case fidColumn:
				// Read fid
				id, ok := vals[i].(int64)
				if !ok {
					return fmt.Errorf("expected int64 fid for column %s, got %T", colName, vals[i])
				}
				fid, fidFound = id, true
			case source.Table.gcolumn:
				// Skip geometry column
			default:
				// Interpret data
				v, ok := cleanData(vals[i])
				if !ok {
					return fmt.Errorf("unexpected type for sqlite column data: %v: %T", colName, v)
				}
				row[colName] = v
			}
		}
		if !fidFound {
			return fmt.Errorf("row did not contain fid column %s", fidColumn)
		}
		attributes[fid] = row
	}
	return rows.Err()
}

func cleanData(v any) (any, bool) {
	switch v := v.(type) {
	case []uint8:
		asBytes := make([]byte, len(v))
		copy(asBytes, v)
		return v, true
	case int64:
		return v, true
	case float64:
		return v, true
	case time.Time:
		return v.Format(time.RFC3339), true
	case string:
		return v, true
	case nil:
		return nil, true
	default:
		return nil, false
	}
}
