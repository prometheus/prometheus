package govalidator

type Errors []error

func (es Errors) Errors() []error {
	return es
}

func (es Errors) Error() string {
	var err string
	for _, e := range es {
		err += e.Error() + ";"
	}
	return err
}

type Error struct {
	Name                     string
	Err                      error
	CustomErrorMessageExists bool
}

func (e Error) Error() string {
	if e.CustomErrorMessageExists {
		return e.Err.Error()
	} else {
		return e.Name + ": " + e.Err.Error()
	}
}
