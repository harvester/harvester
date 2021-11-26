package util

import (
	"bufio"
	"bytes"
	"errors"
	"os"
	"path/filepath"

	"github.com/sirupsen/logrus"
)

const (
	LogComponentField = "component"
)

type LonghornFormatter struct {
	*logrus.TextFormatter

	LogsDir string

	logFiles []*os.File
}

type LonghornWriter struct {
	file *os.File
	name string
	path string
}

func NewLonghornWriter(name string, logsDir string) (*LonghornWriter, error) {
	logPath := filepath.Join(logsDir, name+".log")
	logPath, err := filepath.Abs(logPath)
	if err != nil {
		return nil, err
	}
	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, err
	}
	return &LonghornWriter{
		file: file,
		name: name,
		path: logPath,
	}, nil
}

func SetUpLogger(logsDir string) error {
	if err := os.MkdirAll(logsDir, 0755); err != nil {
		return err
	}
	logsDir, err := filepath.Abs(logsDir)
	if err != nil {
		return err
	}
	testFile := filepath.Join(logsDir, "test")
	if _, err := os.OpenFile(testFile, os.O_WRONLY|os.O_CREATE, 0644); os.IsPermission(err) {
		return err
	}
	logrus.Infof("Storing process logs at path: %v", logsDir)
	logrus.SetFormatter(LonghornFormatter{
		TextFormatter: &logrus.TextFormatter{
			DisableColors: false,
		},
		LogsDir: logsDir,
	})
	return nil
}

func (l LonghornFormatter) Format(entry *logrus.Entry) ([]byte, error) {
	logMsg := &bytes.Buffer{}
	component, ok := entry.Data[LogComponentField]
	if !ok {
		component = "longhorn-instance-manager"
	}
	component, ok = component.(string)
	if !ok {
		return nil, errors.New("field component must be a string")
	}
	logMsg.WriteString("[" + component.(string) + "] ")
	if component == "longhorn-instance-manager" {
		msg, err := l.TextFormatter.Format(entry)
		if err != nil {
			return nil, err
		}
		logMsg.Write(msg)
	} else {
		logMsg.WriteString(entry.Message)
	}

	return logMsg.Bytes(), nil
}

func (l LonghornWriter) Close() error {
	if err := l.file.Close(); err != nil {
		return err
	}
	return nil
}

func (l LonghornWriter) StreamLog(done chan struct{}) (chan string, error) {
	file, err := os.OpenFile(l.path, os.O_RDONLY, 0644)
	if err != nil {
		return nil, err
	}
	logChan := make(chan string)
	scanner := bufio.NewScanner(file)
	go func() {
		for scanner.Scan() {
			select {
			case <-done:
				close(logChan)
				return
			default:
				logChan <- scanner.Text()
			}
		}
		close(logChan)
		file.Close()
	}()
	return logChan, nil
}

func (l LonghornWriter) Write(input []byte) (int, error) {
	msg := string(input)
	logrus.WithField(LogComponentField, l.name).Println(msg)
	outLen, err := l.file.Write(input)
	if err != nil {
		return 0, err
	}
	if err := l.file.Sync(); err != nil {
		return 0, err
	}
	return outLen, nil
}
