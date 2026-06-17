package dataflow

import (
	"reflect"
	"sync"

	"github.com/go-playground/locales/en"
	ut "github.com/go-playground/universal-translator"
	en_translations "github.com/go-playground/validator/v10/translations/en"

	"github.com/go-playground/validator/v10"
)

var valid *defaultValidator = &defaultValidator{}

type defaultValidator struct {
	once     sync.Once
	validate *validator.Validate
	trans    ut.Translator
}

func (v *defaultValidator) ValidateStruct(obj interface{}) error {

	if kindOfData(obj) == reflect.Struct {

		v.lazyinit()

		if err := v.validate.Struct(obj); err != nil {
			return err
		}
	}

	return nil
}

func (v *defaultValidator) Engine() interface{} {
	v.lazyinit()
	return v.validate
}

func (v *defaultValidator) lazyinit() {
	v.once.Do(func() {
		en := en.New()
		uni := ut.New(en, en)

		v.validate = validator.New()
		v.validate.SetTagName("valid")
		v.trans, _ = uni.GetTranslator("en")
		err := en_translations.RegisterDefaultTranslations(v.validate, v.trans)
		if err != nil {
			panic(err)
		}

		v.validate.RegisterTagNameFunc(func(fld reflect.StructField) string {
			return "error: " + fld.Name
		})

		err = v.validate.RegisterTranslation("required", v.trans, func(ut ut.Translator) error {
			return ut.Add("required", "{0} must have a value!", true) // see universal-translator for details
		}, func(ut ut.Translator, fe validator.FieldError) string {
			t, _ := ut.T("required", fe.Field())

			return t
		})
		if err != nil {
			panic(err)
		}

		// add any custom validations etc. here
	})
}

func kindOfData(data interface{}) reflect.Kind {

	value := reflect.ValueOf(data)
	valueType := value.Kind()

	if valueType == reflect.Ptr {
		valueType = value.Elem().Kind()
	}
	return valueType
}
