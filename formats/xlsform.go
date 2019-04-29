package formats

import (
	"fmt"
	"path/filepath"
	"reflect"

	"github.com/extrame/xls"
	"github.com/tealeg/xlsx"
)

type XlsForm struct {
	Survey   []SurveyRow
	Choices  []ChoicesRow
	FileName string
}
type SurveyRow struct {
	Type, Name, Label,
	Relevant, Constraint, Calculation, Required, RepeatCount string
	LineNumber int
}
type ChoicesRow struct {
	ListName, Name, Label string
	LineNumber            int
}

// Defines which sheets/columns to read from an excel file.
// Names must appear in the same order as the fields of XlsForm.
var sheetInfos = []sheetInfo{
	{
		name:      "survey",
		mandatory: true,
		columns: []columnInfo{
			{name: "type", mandatory: true},
			{name: "name", mandatory: true},
			{name: "label", mandatory: true},
			{name: "relevant"},
			{name: "constraint"},
			{name: "calculation"},
			{name: "required"},
			{name: "repeat_count"},
		},
	}, {
		name:      "choices",
		mandatory: true,
		columns: []columnInfo{
			{name: "list name", mandatory: true},
			{name: "name", mandatory: true},
			{name: "label", mandatory: true},
		},
	},
}

type sheetInfo struct {
	name      string
	mandatory bool
	columns   []columnInfo
}
type columnInfo struct {
	name      string
	mandatory bool
}

func DecXlsFromFile(filePath string) (*XlsForm, error) {
	_, fileName := filepath.Split(filePath)
	wb, err := readWorkBook(filePath)
	if err != nil {
		return nil, fmt.Errorf("Could not read excel file %s: %s", fileName, err)
	}

	form := XlsForm{FileName: fileName}
	formVal := reflect.ValueOf(&form).Elem()
	for s, sheetInfo := range sheetInfos {
		rows := wb.Rows(sheetInfo.name)
		if rows == nil && sheetInfo.mandatory {
			return nil, fmt.Errorf("Missing mandatory sheet %q in file %s", sheetInfo.name, fileName)
		}
		if rows == nil {
			continue // not mandatory, skip
		}
		headIndex := firstNonempty(rows)
		if headIndex == -1 {
			return nil, fmt.Errorf("Empty sheet %q in file %s", sheetInfo.name, fileName)
		}
		head := rows[headIndex]
		colIndices := make([]int, len(sheetInfo.columns))
		for j, colInfo := range sheetInfo.columns {
			colIndices[j] = indexOfString(head, colInfo.name)
			if colIndices[j] == -1 && colInfo.mandatory {
				return nil, fmt.Errorf("Error in file %s, sheet %q: column %q is mandatory",
					fileName, sheetInfo.name, colInfo.name)
			}
		}
		destSlice := formVal.Field(s)
		for i := headIndex + 1; i < len(rows); i++ {
			row := rows[i]
			if isEmpty(row) {
				continue
			}
			destRow := reflect.New(destSlice.Type().Elem()).Elem()
			destRow.FieldByName("LineNumber").Set(reflect.ValueOf(i + 1))
			for j := range sheetInfo.columns {
				if colIndices[j] != -1 {
					destRow.Field(j).Set(reflect.ValueOf(row[colIndices[j]]))
				}
			}
			destSlice.Set(reflect.Append(destSlice, destRow))
		}
	}
	return &form, nil
}

type workBook interface {
	Rows(sheetName string) [][]string
}

type xlsxWorkBook xlsx.File

func (wb *xlsxWorkBook) Rows(sheetName string) [][]string {
	sheet, ok := wb.Sheet[sheetName]
	if !ok {
		return nil
	}
	rows := make([][]string, sheet.MaxRow+1)
	numCols := sheet.MaxCol + 1
	for i := range rows {
		rows[i] = make([]string, numCols)
		for j := range rows[i] {
			rows[i][j] = sheet.Cell(i, j).Value
		}
	}
	return rows
}

type xlsWorkBook xls.WorkBook

func (wb *xlsWorkBook) Rows(sheetName string) [][]string {
	var sheet *xls.WorkSheet
	for i := 0; i < (*xls.WorkBook)(wb).NumSheets(); i++ {
		if s := (*xls.WorkBook)(wb).GetSheet(i); s.Name == sheetName {
			sheet = s
			break
		}
	}
	if sheet == nil {
		return nil
	}
	rows := make([][]string, sheet.MaxRow+1)
	numCols := 0
	for i := range rows {
		if row := sheet.Row(i); row != nil && row.LastCol()+1 > numCols {
			numCols = row.LastCol() + 1
		}
	}
	for i := range rows {
		rows[i] = make([]string, numCols)
		row := sheet.Row(i)
		if row == nil {
			continue
		}
		for j := range rows[i] {
			rows[i][j] = row.Col(j)
		}
	}
	return rows
}

func readWorkBook(fileName string) (workBook, error) {
	switch ext := filepath.Ext(fileName); ext {
	case ".xls":
		wb, err := xls.Open(fileName, "utf-8")
		return (*xlsWorkBook)(wb), err
	case ".xlsx":
		f, err := xlsx.OpenFile(fileName)
		return (*xlsxWorkBook)(f), err
	default:
		return nil, fmt.Errorf("Unsupported excel file type: %s", ext)
	}
	// Not sure if the libraries close the files themselves or
	// if/how we are supposed to do it.
}

func isEmpty(row []string) bool {
	for _, cell := range row {
		if cell != "" {
			return false
		}
	}
	return true
}

func firstNonempty(rows [][]string) int {
	for i, row := range rows {
		if !isEmpty(row) {
			return i
		}
	}
	return -1
}

func indexOfString(row []string, name string) int {
	for i, cell := range row {
		if cell == name {
			return i
		}
	}
	return -1
}
