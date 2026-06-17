package informer

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/rancher/apiserver/pkg/types"
	"github.com/rancher/steve/pkg/sqlcache/db"
	"github.com/rancher/steve/pkg/sqlcache/partition"
	"github.com/rancher/steve/pkg/sqlcache/sqltypes"
	"github.com/sirupsen/logrus"
)

func (l *ListOptionIndexer) ListSummaryFields(ctx context.Context, lo *sqltypes.ListOptions, partitions []partition.Partition, dbName string, namespace string) (*types.APISummary, error) {
	joinTableIndexByLabelName := make(map[string]int)
	const mainFieldPrefix = "f1"
	const isSummaryFilter = true
	includeSort := lo.SummaryFieldList != nil
	filterComponents, err := l.compileQuery(lo, partitions, namespace, dbName, mainFieldPrefix, joinTableIndexByLabelName, includeSort, isSummaryFilter)
	if err != nil {
		return nil, err
	}
	summaryNamespaced := lo.SummaryNamespaced
	unsortedSummary := types.APISummary{SummaryItems: make([]types.SummaryEntry, 0)}
	var copyOfJoinTableIndexByLabelName map[string]int
	var copyOfFilterComponents filterComponentsT
	// We have to copy the current data-structures because processing other label summary-fields
	// could modify them, but we don't want to see those changes on subsequent fields
	for fieldNum, field := range lo.SummaryFieldList {
		// Don't make copies on the last run
		if fieldNum == len(lo.SummaryFieldList)-1 {
			copyOfJoinTableIndexByLabelName = joinTableIndexByLabelName
			// This does a shallow copy, which is fine.  filterComponents.copy() does a deep copy
			copyOfFilterComponents = *filterComponents
		} else {
			copyOfJoinTableIndexByLabelName = make(map[string]int)
			for k, v := range joinTableIndexByLabelName {
				copyOfJoinTableIndexByLabelName[k] = v
			}
			copyOfFilterComponents = filterComponents.copy()
		}
		err := l.ListSummaryForField(ctx, field, fieldNum, dbName, &copyOfFilterComponents, mainFieldPrefix, copyOfJoinTableIndexByLabelName, summaryNamespaced, &unsortedSummary)
		if err != nil {
			return nil, err
		}
	}
	return sortSummaries(&unsortedSummary), nil
}

func sortSummaries(pUnsortedSummary *types.APISummary) *types.APISummary {
	sortedItems := slices.SortedFunc(slices.Values(pUnsortedSummary.SummaryItems), func(a, b types.SummaryEntry) int {
		return strings.Compare(strings.ToLower(a.Property), strings.ToLower(b.Property))
	})
	return &types.APISummary{SummaryItems: sortedItems}
}

func (l *ListOptionIndexer) ListSummaryForField(ctx context.Context, field []string, fieldNum int, dbName string, filterComponents *filterComponentsT, mainFieldPrefix string, joinTableIndexByLabelName map[string]int, summaryNamespaced bool, pUnsortedSummary *types.APISummary) error {
	queryInfo, err := l.constructSummaryQueryForField(field, fieldNum, dbName, filterComponents, mainFieldPrefix, joinTableIndexByLabelName, summaryNamespaced)
	if err != nil {
		return err
	}
	logrus.Debugf("Summary ListOptionIndexer prepared statement: %v", queryInfo.query)
	logrus.Debugf("Params: %v", queryInfo.params)
	items, err := l.executeSummaryQueryForField(ctx, queryInfo, field, summaryNamespaced)
	if err != nil {
		return err
	}
	return populateSummaryObject(items, summaryNamespaced, pUnsortedSummary)
}

func (l *ListOptionIndexer) constructSummaryQueryForField(fieldParts []string, fieldNum int, dbName string, filterComponents *filterComponentsT, mainFieldPrefix string, joinTableIndexByLabelName map[string]int, summaryNamespaced bool) (*QueryInfo, error) {
	columnName := toColumnName(fieldParts)
	var columnNameToDisplay string
	var err error
	if isLabelsFieldList(fieldParts) {
		columnNameToDisplay, err = getLabelColumnNameToDisplay(fieldParts)
	} else {
		columnNameToDisplay, err = l.getStandardColumnNameToDisplay(fieldParts, mainFieldPrefix)
	}
	if err != nil {
		return nil, err
	}
	if filterComponents.isEmpty && !summaryNamespaced {
		if !isLabelsFieldList(fieldParts) {
			// No need for a main-field prefix, so recalc
			var err error
			columnNameToDisplay, err = l.getStandardColumnNameToDisplay(fieldParts, "")
			if err != nil {
				//TODO: Prove that this can't happen
				return nil, err
			}
		}
		return l.constructSimpleSummaryQueryForField(fieldParts, dbName, columnName, columnNameToDisplay)
	}
	return l.constructComplexSummaryQueryForField(fieldParts, fieldNum, dbName, columnName, columnNameToDisplay, filterComponents, mainFieldPrefix, joinTableIndexByLabelName, summaryNamespaced)
}

func (l *ListOptionIndexer) constructSimpleSummaryQueryForField(fieldParts []string, dbName, columnName, columnNameToDisplay string) (*QueryInfo, error) {
	if isLabelsFieldList(fieldParts) {
		return l.constructSimpleSummaryQueryForLabelField(fieldParts, dbName, columnName, columnNameToDisplay)
	}
	return l.constructSimpleSummaryQueryForStandardField(fieldParts, dbName, columnName, columnNameToDisplay)
}

func (l *ListOptionIndexer) constructSimpleSummaryQueryForLabelField(fieldParts []string, dbName, columnName, columnNameToDisplay string) (*QueryInfo, error) {
	query := fmt.Sprintf(`SELECT '%s' AS p, COUNT(*) AS c, value AS k
	FROM "%s_labels"
	WHERE label = ? AND k != ""
	GROUP BY k`,
		columnNameToDisplay, dbName)
	args := make([]any, 1)
	args[0] = fieldParts[2]
	return &QueryInfo{query: query, params: args}, nil
}

func (l *ListOptionIndexer) constructSimpleSummaryQueryForStandardField(fieldParts []string, dbName, columnName, columnNameToDisplay string) (*QueryInfo, error) {
	query := fmt.Sprintf(`SELECT '%s' AS p, COUNT(*) AS c, %s AS k
	FROM "%s_fields"
	WHERE k != ""
	GROUP BY k`,
		columnName, columnNameToDisplay, dbName)
	return &QueryInfo{query: query}, nil
}

func (l *ListOptionIndexer) executeSummaryQueryForField(ctx context.Context, queryInfo *QueryInfo, field []string, summaryNamespaced bool) ([][]string, error) {
	stmt, err := l.Prepare(queryInfo.query)
	if err != nil {
		return nil, err
	}
	params := queryInfo.params
	defer func() {
		if cerr := stmt.Close(); cerr != nil && err == nil {
			err = errors.Join(err, cerr)
		}
	}()

	var items [][]string
	err = l.WithTransaction(ctx, false, func(tx db.TxClient) error {
		now := time.Now()
		rows, err := tx.Stmt(stmt).QueryContext(ctx, params...)
		if err != nil {
			return err
		}
		elapsed := time.Since(now)
		logLongQuery(elapsed, queryInfo.query, params)
		items, err = l.ReadStringIntString1or2(rows, summaryNamespaced)
		if err != nil {
			return fmt.Errorf("executeSummaryQueryForField: read objects: %w", err)
		}
		return nil
	})
	return items, err
}

func populateSummaryObject(items [][]string, summaryNamespaced bool, pUnsortedSummary *types.APISummary) error {
	if summaryNamespaced {
		return populateNamespacedSummaryObject(items, pUnsortedSummary)
	}
	entries := make(map[string]types.SummaryEntry)
	for _, item := range items {
		var summaryEntry types.SummaryEntry
		var ok bool

		propertyName := item[0]
		val, err := strconv.Atoi(item[1])
		if err != nil {
			return err
		}
		propertyValue := item[2]
		summaryEntry, ok = entries[propertyName]
		if !ok {
			summaryEntry = types.SummaryEntry{
				Property: propertyName,
				Counts:   make(map[string]types.SummaryWithBreakdown),
			}
			entries[propertyName] = summaryEntry
		}
		entries[propertyName].Counts[propertyValue] = types.SummaryWithBreakdown{Total: val}
	}
	pUnsortedSummary.SummaryItems = append(pUnsortedSummary.SummaryItems, slices.Collect(maps.Values(entries))...)
	return nil
}

func populateNamespacedSummaryObject(items [][]string, pUnsortedSummary *types.APISummary) error {
	entries := make(map[string]types.SummaryEntry)
	for _, item := range items {
		var summaryEntry types.SummaryEntry
		var ok bool

		propertyName := item[0]
		val, err := strconv.Atoi(item[1])
		if err != nil {
			return err
		}
		propertyValue := item[2]
		namespace := item[3]

		summaryEntry, ok = entries[propertyName]
		if !ok {
			summaryEntry = types.SummaryEntry{
				Property: propertyName,
				Counts:   make(map[string]types.SummaryWithBreakdown),
			}
			entries[propertyName] = summaryEntry
		}

		swb := summaryEntry.Counts[propertyValue]
		if swb.Namespace == nil {
			swb.Namespace = make(map[string]int)
		}
		swb.Total += val
		swb.Namespace[namespace] += val
		summaryEntry.Counts[propertyValue] = swb
	}

	pUnsortedSummary.SummaryItems = append(pUnsortedSummary.SummaryItems, slices.Collect(maps.Values(entries))...)
	return nil
}
