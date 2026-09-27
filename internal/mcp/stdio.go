package mcp

import (
	"bufio"
	"context"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"
)

type stdioTransport struct {
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	scanner *bufio.Scanner
}

func startStdio(spec ServerSpec) (*stdioTransport, error) {
	command := exec.Command(spec.Command, spec.Args...)
	if len(spec.Env) > 0 {
		environment := os.Environ()
		for key, value := range spec.Env {
			environment = append(environment, key+"="+value)
		}
		command.Env = environment
	}
	stdin, err := command.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := command.StderrPipe()
	if err != nil {
		return nil, err
	}
	if err := command.Start(); err != nil {
		return nil, err
	}
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line != "" {
				log.Printf("mcp[%s]: %s", spec.Name, line)
			}
		}
	}()
	transport := &stdioTransport{cmd: command, stdin: stdin}
	transport.scanner = bufio.NewScanner(stdout)
	transport.scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	return transport, nil
}

func (s *stdioTransport) Send(ctx context.Context, data []byte) error {
	_, err := s.stdin.Write(append(data, '\n'))
	return err
}

func (s *stdioTransport) Receive(ctx context.Context) ([]byte, error) {
	if !s.scanner.Scan() {
		if err := s.scanner.Err(); err != nil {
			return nil, err
		}
		return nil, io.EOF
	}
	return append([]byte(nil), s.scanner.Bytes()...), nil
}

func (s *stdioTransport) Close() error {
	_ = s.stdin.Close()
	if s.cmd.Process != nil {
		return s.cmd.Process.Kill()
	}
	return nil
}
