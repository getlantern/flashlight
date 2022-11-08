/*
	MIT License

	Copyright (c) 2020 Operator Foundation

	Permission is hereby granted, free of charge, to any person obtaining a copy
	of this software and associated documentation files (the "Software"), to deal
	in the Software without restriction, including without limitation the rights
	to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
	copies of the Software, and to permit persons to whom the Software is
	furnished to do so, subject to the following conditions:

	The above copyright notice and this permission notice shall be included in all
	copies or substantial portions of the Software.

	THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
	IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
	FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
	AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
	LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
	OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
	SOFTWARE.
*/

package replicant

import (
	"encoding/json"

	"github.com/OperatorFoundation/Replicant-go/Replicant/v3/polish"
	"github.com/OperatorFoundation/Replicant-go/Replicant/v3/toneburst"
)

type ClientConfig struct {
	Toneburst toneburst.Config
	Polish    polish.ClientConfig
	Address   string
}

type ServerConfig struct {
	Toneburst toneburst.Config
	Polish    polish.ServerConfig
}

type ClientJSONConfig struct {
	Config string `json:"config"`
}

type ServerJSONInnerConfig struct {
	Config string `json:"config"`
}

type ServerJSONOuterConfig struct {
	Replicant ServerJSONInnerConfig
}

func (config ServerConfig) Marshal() (string, error) {
	configString, configStringError := config.Encode()
	if configStringError != nil {
		return "", configStringError
	}

	innerConfig := ServerJSONInnerConfig{Config: configString}
	outerConfig := ServerJSONOuterConfig{Replicant: innerConfig}

	configBytes, marshalError := json.Marshal(outerConfig)
	if marshalError != nil {
		return "", marshalError
	}

	return string(configBytes), nil
}

func (config ClientConfig) Marshal() (string, error) {

	configString, configStringError := config.Encode()
	if configStringError != nil {
		return "", configStringError
	}

	clientConfig := ClientJSONConfig{Config: configString}

	configBytes, marshalError := json.Marshal(clientConfig)
	if marshalError != nil {
		return "", marshalError
	}

	return string(configBytes), nil
}
