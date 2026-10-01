package mcp

import (
	"bufio"
	"context"
	"errors"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

type stdioTransport struct {
	command    *exec.Cmd
	stdin      io.WriteCloser
	scanner    *bufio.Scanner
	writeGate  chan struct{}
	closeOnce  sync.Once
	closeError error
}

func startStdioTransport(serverSpec ServerSpec) (*stdioTransport, error) {
	command := exec.Command(serverSpec.Command, serverSpec.Args...)
	if len(serverSpec.Env) > 0 {
		environment := os.Environ()
		for key, value := range serverSpec.Env {
			environment = append(environment, key+"="+value)
		}
		command.Env = environment
	}
	stdin, operationError := command.StdinPipe()
	if operationError != nil {
		return nil, operationError
	}
	stdout, operationError := command.StdoutPipe()
	if operationError != nil {
		return nil, operationError
	}
	stderr, operationError := command.StderrPipe()
	if operationError != nil {
		return nil, operationError
	}
	if operationError := command.Start(); operationError != nil {
		return nil, operationError
	}
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line != "" {
				log.Printf("mcp[%s]: %s", serverSpec.Name, line)
			}
		}
	}()
	transport := &stdioTransport{command: command, stdin: stdin, writeGate: make(chan struct{}, 1)}
	transport.scanner = bufio.NewScanner(stdout)
	transport.scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	return transport, nil
}

func (stdioTransport *stdioTransport) Send(operationContext context.Context, data []byte) error {
	if operationError := operationContext.Err(); operationError != nil {
		return operationError
	}
	select {
	case stdioTransport.writeGate <- struct{}{}:
	case <-operationContext.Done():
		return operationContext.Err()
	}
	defer func() {
		<-stdioTransport.writeGate
	}()
	pipe := stdioTransport.stdin.(*os.File)
	if deadline, found := operationContext.Deadline(); found {
		if operationError := pipe.SetWriteDeadline(deadline); operationError != nil {
			return operationError
		}
	}
	applied := make(chan struct{})
	stop := context.AfterFunc(operationContext, func() {
		_ = pipe.SetWriteDeadline(time.Now())
		close(applied)
	})
	defer func() {
		if !stop() {
			<-applied
		}
		_ = pipe.SetWriteDeadline(time.Time{})
	}()
	_, operationError := stdioTransport.stdin.Write(append(data, '\n'))
	return operationError
}

func (stdioTransport *stdioTransport) Receive(operationContext context.Context) ([]byte, error) {
	if !stdioTransport.scanner.Scan() {
		if operationError := stdioTransport.scanner.Err(); operationError != nil {
			return nil, operationError
		}
		return nil, io.EOF
	}
	return append([]byte(nil), stdioTransport.scanner.Bytes()...), nil
}

func (stdioTransport *stdioTransport) Close() error {
	stdioTransport.closeOnce.Do(func() {
		_ = stdioTransport.stdin.Close()
		if stdioTransport.command.Process != nil {
			_ = stdioTransport.command.Process.Kill()
		}
		operationError := stdioTransport.command.Wait()
		var exitError *exec.ExitError
		if operationError != nil && !errors.As(operationError, &exitError) {
			stdioTransport.closeError = operationError
		}
	})
	return stdioTransport.closeError
}
