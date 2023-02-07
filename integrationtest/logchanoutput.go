package main

import "fmt"

type GologChanOutput struct {
	LogsChan chan string
}

func NewGologChanOutput() *GologChanOutput {
	return &GologChanOutput{LogsChan: make(chan string)}
}

func (o *GologChanOutput) Debug(
	prefix string,
	skipFrames int,
	printStack bool,
	severity string,
	arg interface{},
	values map[string]interface{},
) {
	o.LogsChan <- fmt.Sprintf("DEBUG: %s: %s", prefix, arg)
}

func (o *GologChanOutput) Error(
	prefix string,
	skipFrames int,
	printStack bool,
	severity string,
	arg interface{},
	values map[string]interface{},
) {
	o.LogsChan <- fmt.Sprintf("ERROR: %s: %s", prefix, arg)
}
