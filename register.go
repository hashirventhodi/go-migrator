package main

import (
	"sync"
)

var (
	modelLock sync.Mutex
)

func RegisterModels(models ...interface{}) {
	modelLock.Lock()
	defer modelLock.Unlock()

	models = append(models, models...)
}
