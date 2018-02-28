package reflect2

type safeStructType struct {
	safeType
}

func (type2 *safeStructType) FieldByName(name string) StructField {
	field, found := type2.Type.FieldByName(name)
	if !found {
		panic("field " + name + " not found")
	}
	return &safeField{StructField: field}
}