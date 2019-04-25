package formats

import (
	"errors"
	"fmt"
	"strings"
)

func Xls2ajf(xls *XlsForm) (*AjfForm, error) {
	var ajf AjfForm
	var choicesMap map[string][]Choice
	ajf.ChoicesOrigins, choicesMap = buildChoicesOrigins(xls.Choices)

	survey, err := preprocessGroups(xls.Survey)
	if err != nil {
		return nil, err
	}
	global, err := buildGroup(survey)
	if err != nil {
		return nil, err
	}
	err = checkChoicesRef(&global, choicesMap)
	if err != nil {
		return nil, err
	}
	ajf.Slides = global.Nodes
	for i := range ajf.Slides {
		ajf.Slides[i].Type = NtSlide
	}
	assignIds(ajf.Slides, 0)
	return &ajf, nil
}

func buildChoicesOrigins(rows []ChoicesRow) ([]ChoicesOrigin, map[string][]Choice) {
	choicesMap := make(map[string][]Choice)
	for _, row := range rows {
		choicesMap[row.ListName] = append(choicesMap[row.ListName], Choice{
			Value: row.Name,
			Label: row.Label,
		})
	}
	var co []ChoicesOrigin
	for name, list := range choicesMap {
		co = append(co, ChoicesOrigin{
			Type:        OtFixed,
			Name:        name,
			ChoicesType: CtString,
			Choices:     list,
		})
	}
	return co, choicesMap
}

var notBalancedErr = errors.New("Groups are not balanced")

func preprocessGroups(survey []SurveyRow) ([]SurveyRow, error) {
	groupDepth := 0
	ungroupedItems := false
	for _, row := range survey {
		switch row.Type {
		case beginGroup:
			groupDepth++
		case endGroup:
			groupDepth--
			if groupDepth < 0 {
				return nil, notBalancedErr
			}
		default:
			if groupDepth == 0 {
				ungroupedItems = true
			}
		}
	}
	if groupDepth != 0 {
		return nil, notBalancedErr
	}
	if ungroupedItems {
		// Wrap everything into a slide.
		survey = append([]SurveyRow{{Type: beginGroup, Name: "form", Label: "Form"}}, survey...)
		survey = append(survey, SurveyRow{Type: endGroup})
	}
	// Wrap everything into a global phony group,
	// it allows building the form with a single call to buildGroup.
	survey = append([]SurveyRow{{Type: beginGroup, Name: "global", Label: "Global"}}, survey...)
	survey = append(survey, SurveyRow{Type: endGroup})
	return survey, nil
}

func buildGroup(survey []SurveyRow) (Node, error) {
	if survey[0].Type != beginGroup {
		panic("not a group")
	}
	group := Node{
		Name:  survey[0].Name,
		Label: survey[0].Label,
		Type:  NtGroup,
		Nodes: make([]Node, 0),
	}
	for i := 1; i < len(survey); i++ {
		row := survey[i]
		switch {
		case row.Type == beginGroup:
			end := groupEnd(survey, i)
			child, err := buildGroup(survey[i:end])
			if err != nil {
				return Node{}, err
			}
			group.Nodes = append(group.Nodes, child)
			i = end - 1
		case row.Type == endGroup:
			if i != len(survey)-1 {
				panic("unexpected end of group")
			}
		case isSupportedField(row.Type):
			field := buildField(&row)
			group.Nodes = append(group.Nodes, field)
		case isUnsupportedField(row.Type):
			return Node{}, fmt.Errorf("Field type %q is not supported", row.Type)
		case row.Type == beginRepeat || row.Type == endRepeat:
			return Node{}, fmt.Errorf("Repeats are not supported")
		default:
			return Node{}, fmt.Errorf("Invalid type %q in survey", row.Type)
		}
	}
	return group, nil
}

func groupEnd(survey []SurveyRow, group int) int {
	groupDepth := 1
	for i := group + 1; i < len(survey); i++ {
		switch survey[i].Type {
		case beginGroup:
			groupDepth++
		case endGroup:
			groupDepth--
			if groupDepth == 0 {
				return i + 1
			}
		}
	}
	panic("group end not found")
}

func buildField(row *SurveyRow) Node {
	field := Node{
		Name:  row.Name,
		Label: row.Label,
		Type:  NtField,
	}
	switch {
	case row.Type == "decimal":
		field.FieldType = &FtNumber
	case row.Type == "text":
		field.FieldType = &FtString
	case row.Type == "select_one yes_no":
		field.FieldType = &FtBoolean
	case isSelectOne(row.Type):
		field.FieldType = &FtSingleChoice
		field.ChoicesOriginRef = row.Type[strings.Index(row.Type, " ")+1:]
	case isSelectMultiple(row.Type):
		field.FieldType = &FtMultipleChoice
		field.ChoicesOriginRef = row.Type[strings.Index(row.Type, " ")+1:]
	case row.Type == "note":
		field.FieldType = &FtNote
		field.HTML = row.Label
	case row.Type == "date":
		field.FieldType = &FtDate
	case row.Type == "time":
		field.FieldType = &FtTime
	case row.Type == "calculate":
		field.FieldType = &FtFormula
	case isUnsupportedField(row.Type):
		panic("unsupported row type: " + row.Type)
	default:
		panic("unrecognized row type: " + row.Type)
	}
	if row.Required == "yes" {
		field.Validation = &FieldValidation{NotEmpty: true}
	}
	return field
}

func checkChoicesRef(node *Node, choicesMap map[string][]Choice) error {
	if node.Type == NtField && (*node.FieldType == FtSingleChoice || *node.FieldType == FtMultipleChoice) {
		if _, ok := choicesMap[node.ChoicesOriginRef]; !ok {
			return fmt.Errorf("Invalid single or multiple choice %q", node.ChoicesOriginRef)
		}
	}
	for i := range node.Nodes {
		err := checkChoicesRef(&node.Nodes[i], choicesMap)
		if err != nil {
			return err
		}
	}
	return nil
}

const idMultiplier = 1000

func assignIds(nodes []Node, parent int) {
	if len(nodes) == 0 {
		return
	}
	nodes[0].Previous = parent
	nodes[0].Id = parent*idMultiplier + 1
	assignIds(nodes[0].Nodes, nodes[0].Id)
	for i := 1; i < len(nodes); i++ {
		nodes[i].Previous = nodes[i-1].Id
		nodes[i].Id = nodes[i-1].Id + 1
		assignIds(nodes[i].Nodes, nodes[i].Id)
	}
}

const (
	beginGroup  = "begin group"
	endGroup    = "end group"
	beginRepeat = "begin repeat"
	endRepeat   = "end repeat"
)

var supportedField = map[string]bool{
	"decimal": true, "text": true, "select_one yes_no": true, "note": true,
	"date": true, "time": true, "calculate": true,
}

func isSupportedField(typ string) bool {
	return supportedField[typ] || isSelectOne(typ) || isSelectMultiple(typ)
}
func isSelectOne(typ string) bool {
	return strings.HasPrefix(typ, "select_one ") && typ != "select_one yes_no"
}
func isSelectMultiple(typ string) bool { return strings.HasPrefix(typ, "select_multiple ") }

var unsupportedField = map[string]bool{
	"integer": true, "range": true, "geopoint": true, "geotrace": true, "geoshape": true,
	"datetime": true, "image": true, "audio": true, "video": true, "file": true,
	"barcode": true, "acknowledge": true, "hidden": true, "xml-external": true,
	// metadata:
	"start": true, "end": true, "today": true, "deviceid": true, "subscriberid": true,
	"simserial": true, "phonenumber": true, "username": true, "email": true,
}

func isUnsupportedField(typ string) bool { return unsupportedField[typ] || isRank(typ) }
func isRank(typ string) bool             { return strings.HasPrefix(typ, "rank ") }
