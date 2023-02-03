package toneburst

import (
	"errors"
	"net"
	"time"

	"github.com/OperatorFoundation/ghostwriter-go"
)

type StarburstConfig struct {
	FunctionName string
}

type StarburstSMTP struct {
}

type StarburstSMTPServer struct {
	StarburstSMTP
}

type StarburstSMTPClient struct {
	StarburstSMTP
}

func (config StarburstConfig) Construct() (ToneBurst, error) {
	switch config.FunctionName {
	case "SMTPServer":
		return &StarburstSMTPServer{}, nil
	case "SMTPClient":
		return &StarburstSMTPClient{}, nil
	default:
		return nil, errors.New("unknown function name")
	}
}

func (smtp *StarburstSMTPServer) Perform(conn net.Conn) error {
	var templateError error
	templateError = smtp.speakTemplate(conn, ghostwriter.Template{String: "220 $1 SMTP service ready\r\n"}, []ghostwriter.Detail{ghostwriter.DetailString{String: "mail.imc.org"}})
	if templateError != nil {
		return templateError
	}

	_, templateError = smtp.listenParse(
		conn,
		ghostwriter.Template{String: "EHLO $1\r\n"},
		[]ghostwriter.ExtractionPattern{
			{Expression: "^([a-zA-Z0-9.-]+)\r",
				Type: ghostwriter.String}},
		253,
		10)
	if templateError != nil {
		return templateError
	}

	templateError = smtp.speakTemplate(conn, ghostwriter.Template{String: "250-$1 offers a warm hug of welcome\r\n250-$2\r\n250-$3\r\n250 $4\r\n"}, []ghostwriter.Detail{ghostwriter.DetailString{String: "mail.imc.org"}, ghostwriter.DetailString{String: "8BITMIME"}, ghostwriter.DetailString{String: "DSN"}, ghostwriter.DetailString{String: "STARTTLS"}})
	if templateError != nil {
		return templateError
	}

	templateError = smtp.listenString(conn, "STARTTLS\r\n")
	if templateError != nil {
		return templateError
	}

	templateError = smtp.speakTemplate(conn, ghostwriter.Template{String: "220 $1\r\n"}, []ghostwriter.Detail{ghostwriter.DetailString{String: "Go ahead"}})
	if templateError != nil {
		return templateError
	}

	return nil
}

func (smtp *StarburstSMTPClient) Perform(conn net.Conn) error {
	_, templateError := smtp.listenParse(
		conn,
		ghostwriter.Template{String: "220 $1 SMTP service ready\r\n"},
		[]ghostwriter.ExtractionPattern{
			{Expression: "^([a-zA-Z0-9.-]+) ",
				Type: ghostwriter.String}},
		253,
		10)
	if templateError != nil {
		return templateError
	}

	templateError = smtp.speakTemplate(conn, ghostwriter.Template{String: "EHLO $1\r\n"}, []ghostwriter.Detail{ghostwriter.DetailString{String: "mail.imc.org"}})
	if templateError != nil {
		return templateError
	}

	_, templateError = smtp.listenParse(
		conn,
		ghostwriter.Template{String: "$1\r\n"},
		[]ghostwriter.ExtractionPattern{{Expression: "250 (STARTTLS)", Type: ghostwriter.String}},
		253,
		10)
	if templateError != nil {
		return templateError
	}

	templateError = smtp.speakString(conn, "STARTTLS\r\n")
	if templateError != nil {
		return templateError
	}

	_, templateError = smtp.listenParse(
		conn,
		ghostwriter.Template{String: "$1\r\n"},
		[]ghostwriter.ExtractionPattern{
			{Expression: "^(.+)\r",
				Type: ghostwriter.String}},
		253,
		10)
	if templateError != nil {
		return templateError
	}

	return nil
}

func (smtp *StarburstSMTP) speakString(connection net.Conn, speakString string) error {
	var bytesWritten = 0
	var writeError error
	var bytesToWrite = []byte(speakString)
	for len(bytesToWrite) > 0 {
		bytesWritten, writeError = connection.Write(bytesToWrite)
		if writeError != nil {
			return writeError
		}
		bytesToWrite = bytesToWrite[bytesWritten:]
	}

	return nil
}

func (smtp *StarburstSMTP) speakTemplate(connection net.Conn, speakTemplate ghostwriter.Template, details []ghostwriter.Detail) error {
	// do ghostwriter string template to get a string, then call speakString
	generated, generateError := ghostwriter.Generate(&speakTemplate, details)
	if generateError != nil {
		return generateError
	}

	smtp.speakString(connection, *generated)

	return nil
}

func (smtp *StarburstSMTP) listenString(connection net.Conn, expected string) error {
	connection.SetReadDeadline(time.Now().Add(5 * time.Minute))
	// use read to get (expected lenght number of)bytes, convert to string, and then compare them to see if they match
	expectedLength := len(expected)
	readBuffer := make([]byte, expectedLength)
	bytesRead, readError := connection.Read(readBuffer)

	if readError != nil {
		return readError
	}

	if bytesRead != expectedLength {
		return errors.New("did not read the expected amount of bytes")
	}

	bufferString := string(readBuffer)
	if bufferString != expected {
		return errors.New("did not read the expected string")
	}

	return nil
}

func (smtp *StarburstSMTP) listenParse(connection net.Conn, template ghostwriter.Template, patterns []ghostwriter.ExtractionPattern, maxSize int, maxTimeoutSeconds int64) ([]ghostwriter.Detail, error) {
	connection.SetReadDeadline(time.Now().Add(5 * time.Minute))
	// keep listening until we have the right number of details (same number as patterns) then return the details
	timeout := time.After(time.Duration(maxTimeoutSeconds) * time.Second)

	var totalBytesRead = 0
	var totalBuffer = make([]byte, 0)

	var byteChannel = make(chan []byte)
	var keepGoingChannel = make(chan bool)

	go func() {
		for <-keepGoingChannel {
			var buffer = make([]byte, 1)
			bytesRead, readError := connection.Read(buffer)
			if readError != nil {
				close(byteChannel)
				return
			}

			if bytesRead == 0 {
				continue
			}

			buffer = buffer[:bytesRead]
			byteChannel <- buffer
		}
	}()

	for totalBytesRead < maxSize {
		keepGoingChannel <- true
		select {
		case <-timeout:
			keepGoingChannel <- false
			return nil, errors.New("listenParse timeout reached")
		case buffer, ok := <-byteChannel:
			if !ok {
				keepGoingChannel <- false
				return nil, errors.New("error while trying to read")
			}
			totalBuffer = append(totalBuffer, buffer...)
			totalBytesRead += len(buffer)

			bufferString := string(totalBuffer)
			details, parseError := ghostwriter.Parse(&template, patterns, bufferString)
			if parseError != nil {
				continue
			}

			if len(details) != len(patterns) {
				continue
			}
			keepGoingChannel <- false
			return details, nil
		}
	}
	return nil, errors.New("listenParse: unexpected code path")
}
