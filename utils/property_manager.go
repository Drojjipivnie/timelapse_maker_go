package utils

import "github.com/magiconair/properties"

var PropertyFiles = []string{"app.properties"}

var Props, _ = properties.LoadFiles(PropertyFiles, properties.UTF8, true)

type PropertyManager struct {
}

func (res PropertyManager) GetProperty(propertyName string) string {
	get, o := Props.Get(propertyName)
	if !o {
		return Props.MustGet(propertyName)
	} else {
		return get
	}
}
