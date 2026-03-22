package main

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/crypto/ssh"
)

var (
	// markdown holds the CV content loaded during startup.
	markdown []byte
	// appConfig stores the validated runtime configuration.
	appConfig *Config

	boldRegex         = regexp.MustCompile(`\*\*(.+?)\*\*`)
	italicRegex       = regexp.MustCompile(`\*(.+?)\*`)
	inlineCodeRegex   = regexp.MustCompile("`([^`]+)`")
	numberedListRegex = regexp.MustCompile(`^(\d+)\. `)
	linkRegex         = regexp.MustCompile(`\[([^\]]+)\]\([^\)]+\)`)
	stringRegex       = regexp.MustCompile(`"([^"\\]|\\.)*"`)
	numberRegex       = regexp.MustCompile(`\b\d+\b`)
)

const (
	// configPathEnv overrides the path to config.yaml.
	configPathEnv = "SSH_CV_CONFIG_PATH"
	// infoPathEnv overrides the path to info.md.
	infoPathEnv = "SSH_CV_INFO_PATH"
	// hostKeyPathEnv overrides the path to the persisted SSH host key.
	hostKeyPathEnv = "SSH_CV_HOST_KEY_PATH"
)

// main loads runtime assets, starts the SSH listener, and serves sessions until shutdown.
func main() {
	var err error

	configPath := resolveRuntimePath(configPathEnv, "config.yaml")
	infoPath := resolveRuntimePath(infoPathEnv, "info.md")

	appConfig, err = loadConfig(configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	markdown, err = os.ReadFile(infoPath)
	switch {
	case os.IsNotExist(err):
		log.Fatalf("Info file not found: %s", infoPath)
	case err != nil:
		log.Fatalf("Failed to read info file %s: %v", infoPath, err)
	case len(markdown) == 0:
		markdown = []byte("No content available.\n")
	}

	hostKeyPath := resolveRuntimePath(hostKeyPathEnv, resolveConfigRelativePath(configPath, appConfig.Server.HostKeyPath))

	log.Printf(
		"Runtime paths resolved: config=%s info=%s host_key=%s",
		logField(configPath),
		logField(infoPath),
		logField(hostKeyPath),
	)

	serverKey, err := loadOrCreateHostKey(hostKeyPath)
	if err != nil {
		log.Fatalf("Failed to initialize host key: %v", err)
	}

	sshConfig := &ssh.ServerConfig{
		NoClientAuth: true,
		NoClientAuthCallback: func(conn ssh.ConnMetadata) (*ssh.Permissions, error) {
			log.Printf(
				"SSH auth accepted: user=%s remote=%s method=%s",
				logField(conn.User()),
				logField(conn.RemoteAddr().String()),
				"none",
			)
			return &ssh.Permissions{}, nil
		},
		AuthLogCallback: func(conn ssh.ConnMetadata, method string, err error) {
			if err != nil {
				log.Printf(
					"SSH auth rejected: user=%s remote=%s method=%s err=%v",
					logField(conn.User()),
					logField(conn.RemoteAddr().String()),
					method,
					err,
				)
				return
			}

			log.Printf(
				"SSH auth attempt: user=%s remote=%s method=%s",
				logField(conn.User()),
				logField(conn.RemoteAddr().String()),
				method,
			)
		},
	}
	sshConfig.AddHostKey(serverKey)

	addr := net.JoinHostPort("0.0.0.0", strconv.Itoa(appConfig.Server.Port))
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("Failed to listen on %s: %v", addr, err)
	}
	defer listener.Close()

	log.Printf("SSH CV server listening on %s", addr)

	connectionLimiter := make(chan struct{}, appConfig.Server.MaxConcurrentConnections)

	signalCh := make(chan os.Signal, 1)
	shutdownCh := make(chan struct{})
	signal.Notify(signalCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-signalCh
		close(shutdownCh)
		_ = listener.Close()
	}()

	for {
		netConn, err := listener.Accept()
		if err != nil {
			select {
			case <-shutdownCh:
				return
			default:
				log.Printf("Accept error: %v", err)
				continue
			}
		}

		select {
		case connectionLimiter <- struct{}{}:
			go handleConnection(netConn, sshConfig, appConfig, connectionLimiter)
		default:
			log.Printf("Connection rejected: concurrency limit reached for remote=%s", logField(netConn.RemoteAddr().String()))
			_ = netConn.Close()
		}
	}
}

// handleConnection performs the SSH handshake and serves the first session channel on the connection.
func handleConnection(netConn net.Conn, sshConfig *ssh.ServerConfig, cfg *Config, connectionLimiter chan struct{}) {
	defer func() { <-connectionLimiter }()
	defer netConn.Close()

	if timeout := time.Duration(cfg.Server.HandshakeTimeoutSeconds) * time.Second; timeout > 0 {
		if err := netConn.SetDeadline(time.Now().Add(timeout)); err != nil {
			log.Printf("Failed to set handshake deadline for remote=%s: %v", logField(netConn.RemoteAddr().String()), err)
			return
		}
	}

	conn, chans, reqs, err := ssh.NewServerConn(netConn, sshConfig)
	if err != nil {
		log.Printf("SSH handshake failed: remote=%s err=%v", logField(netConn.RemoteAddr().String()), err)
		return
	}
	defer conn.Close()

	if err := netConn.SetDeadline(time.Time{}); err != nil {
		log.Printf("Failed to clear handshake deadline for remote=%s: %v", logField(conn.RemoteAddr().String()), err)
	}

	log.Printf("SSH connection established: user=%s remote=%s", logField(conn.User()), logField(conn.RemoteAddr().String()))

	go ssh.DiscardRequests(reqs)

	for newChan := range chans {
		if newChan.ChannelType() != "session" {
			_ = newChan.Reject(ssh.UnknownChannelType, "only session channels are supported")
			continue
		}

		handleSession(newChan, netConn, cfg)
		return
	}
}

// handleSession accepts a single session channel, renders the CV, and waits for the user to disconnect.
func handleSession(newChan ssh.NewChannel, netConn net.Conn, cfg *Config) {
	channel, requests, err := newChan.Accept()
	if err != nil {
		log.Printf("Failed to accept session channel for remote=%s: %v", logField(netConn.RemoteAddr().String()), err)
		return
	}
	defer channel.Close()

	if timeout := time.Duration(cfg.Server.SessionTimeoutSeconds) * time.Second; timeout > 0 {
		if err := netConn.SetDeadline(time.Now().Add(timeout)); err != nil {
			log.Printf("Failed to set session deadline for remote=%s: %v", logField(netConn.RemoteAddr().String()), err)
			return
		}
		defer netConn.SetDeadline(time.Time{})
	}

	go replyToSessionRequests(requests)

	if err := renderSession(channel, cfg); err != nil {
		return
	}

	buf := make([]byte, 1)
	for {
		n, err := channel.Read(buf)
		if n > 0 {
			switch strings.ToLower(string(buf[0])) {
			case "q", "x", "\x03":
				_ = writeString(channel, buildGoodbyeMessage(cfg))
				time.Sleep(200 * time.Millisecond)
				return
			}
		}

		if err != nil {
			return
		}
	}
}

// replyToSessionRequests allows only the request types required for a terminal session.
func replyToSessionRequests(requests <-chan *ssh.Request) {
	for req := range requests {
		allowed := req.Type == "pty-req" || req.Type == "shell" || req.Type == "window-change"
		req.Reply(allowed, nil)
	}
}

// renderSession writes the boot animation, formatted markdown, and exit prompt to the SSH channel.
func renderSession(channel ssh.Channel, cfg *Config) error {
	if err := writeString(channel, "\033[2J\033[H"); err != nil {
		return err
	}
	if err := writeString(channel, "\033[?25l"); err != nil {
		return err
	}

	for _, line := range cfg.Animations.MatrixLines {
		if err := writeString(channel, "\033[32m"+line+"\033[0m\r\n"); err != nil {
			return err
		}
		time.Sleep(time.Duration(cfg.Animations.MatrixSpeedMs) * time.Millisecond)
	}

	if cfg.Animations.ShowScanLines {
		for i := 0; i < cfg.Animations.ScanLinesCount; i++ {
			for j := 0; j <= 50; j++ {
				frame := "\033[32m>" + strings.Repeat("-", j) + "\033[0m\r"
				if err := writeString(channel, frame); err != nil {
					return err
				}
				time.Sleep(time.Duration(cfg.Animations.ScanSpeedMs) * time.Millisecond)
			}
			time.Sleep(30 * time.Millisecond)
		}
	}

	ascii := `

ଘ(੭*ˊᵕˋ)੭* ̀ˋ ɪɴᴛᴇʀɴᴇᴛ

`
	if cfg.Animations.AsciiGlitch {
		glitchFrames := []string{"\033[36m", "\033[35m", "\033[31m", "\033[33m", "\033[32m"}
		for i, char := range ascii {
			if err := writeString(channel, glitchFrames[i%len(glitchFrames)]+string(char)); err != nil {
				return err
			}
			time.Sleep(2 * time.Millisecond)
		}
		if err := writeString(channel, "\033[0m"); err != nil {
			return err
		}
	} else {
		if err := writeString(channel, "\033[36m"+ascii+"\033[0m"); err != nil {
			return err
		}
	}

	statusLines := []string{
		"\r\n\033[32m[OK]\033[0m Initializing interface...",
		"\r\n\033[32m[OK]\033[0m Bypassing firewall...",
		"\r\n\033[32m[OK]\033[0m Decrypting data...",
		"\r\n\033[33m[WAIT]\033[0m Loading data...",
	}
	typewriterSpeed := time.Duration(cfg.Animations.TypewriterSpeedMs) * time.Millisecond
	bootDelay := time.Duration(cfg.Animations.BootDelayMs) * time.Millisecond

	for _, line := range statusLines {
		for _, c := range line {
			if err := writeString(channel, string(c)); err != nil {
				return err
			}
			time.Sleep(typewriterSpeed)
		}
		time.Sleep(bootDelay)
	}

	if err := writeString(channel, "\r\n\r\n\033[32m"); err != nil {
		return err
	}

	matrixRunes := []rune("01アイウエオカキクケコサシスセソタチツテト")
	for i := 0; i < 200; i++ {
		frame := strings.Repeat(" ", i%60) + string(matrixRunes[i%len(matrixRunes)])
		if err := writeString(channel, frame); err != nil {
			return err
		}
		time.Sleep(3 * time.Millisecond)
	}
	if err := writeString(channel, "\033[0m"); err != nil {
		return err
	}

	time.Sleep(200 * time.Millisecond)
	if err := writeString(channel, "\033[2J\033[H"); err != nil {
		return err
	}
	if err := writeString(channel, "\033[?25l"); err != nil {
		return err
	}
	if err := writeString(channel, buildAccessHeader(cfg.Messages.AccessGranted)); err != nil {
		return err
	}

	formatted := formatMarkdown(markdown)
	for _, c := range formatted {
		if err := writeString(channel, string(c)); err != nil {
			return err
		}
		time.Sleep(3 * time.Millisecond)
	}

	if err := writeString(channel, fmt.Sprintf("\r\n\r\n\033[33m%s\033[0m\r\n", cfg.Messages.PressToExit)); err != nil {
		return err
	}

	return writeString(channel, "\033[?25h")
}

// buildAccessHeader creates the banner shown before the CV content.
func buildAccessHeader(message string) string {
	content := "  " + message + "  "
	border := strings.Repeat("═", len([]rune(content)))

	return fmt.Sprintf(
		"\r\n\033[1;32m╔%s╗\033[0m\r\n\033[1;32m║\033[0m\033[1;36m%s\033[0m\033[1;32m║\033[0m\r\n\033[1;32m╚%s╝\033[0m\r\n\r\n",
		border,
		content,
		border,
	)
}

// buildGoodbyeMessage renders the disconnect text displayed before the session closes.
func buildGoodbyeMessage(cfg *Config) string {
	return fmt.Sprintf(
		"\r\n\r\n\033[31m[!]\033[0m \033[1;33m%s\033[0m\r\n\033[36m    %s\033[0m\r\n",
		cfg.Messages.Goodbye,
		cfg.Messages.HackMessage,
	)
}

// loadOrCreateHostKey loads a persisted host key or creates one on first startup.
func loadOrCreateHostKey(path string) (ssh.Signer, error) {
	privateKeyPEM, err := os.ReadFile(path)
	if err == nil {
		if info, statErr := os.Stat(path); statErr == nil && info.Mode().Perm()&0o077 != 0 {
			return nil, fmt.Errorf("host key permissions are too open for %s", path)
		}
		return ssh.ParsePrivateKey(privateKeyPEM)
	}
	if !os.IsNotExist(err) {
		return nil, fmt.Errorf("read host key: %w", err)
	}

	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, fmt.Errorf("create host key directory: %w", err)
		}
	}

	signer, generatedKeyPEM, err := generateServerKey()
	if err != nil {
		return nil, err
	}

	if err := os.WriteFile(path, generatedKeyPEM, 0o600); err != nil {
		return nil, fmt.Errorf("write host key: %w", err)
	}

	return signer, nil
}

// generateServerKey creates an RSA private key and returns both the signer and PEM payload.
func generateServerKey() (ssh.Signer, []byte, error) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, err
	}

	privateKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	})

	signer, err := ssh.ParsePrivateKey(privateKeyPEM)
	if err != nil {
		return nil, nil, err
	}

	return signer, privateKeyPEM, nil
}

// formatMarkdown converts markdown to terminal-friendly text with lightweight ANSI styling.
func formatMarkdown(md []byte) string {
	var buf bytes.Buffer

	lines := strings.Split(string(md), "\n")
	inCodeBlock := false
	var codeLang string
	var codeContent []string

	for _, line := range lines {
		if strings.HasPrefix(line, "```") {
			if inCodeBlock {
				if len(codeContent) > 0 {
					buf.WriteString(highlightCode(strings.Join(codeContent, "\n"), codeLang))
				}
				inCodeBlock = false
				codeContent = nil
				codeLang = ""
			} else {
				inCodeBlock = true
				codeLang = strings.TrimSpace(strings.TrimPrefix(line, "```"))
			}
			continue
		}

		if inCodeBlock {
			codeContent = append(codeContent, line)
			continue
		}

		buf.WriteString(processLine(line))
		buf.WriteString("\r\n")
	}

	if inCodeBlock && len(codeContent) > 0 {
		buf.WriteString(highlightCode(strings.Join(codeContent, "\n"), codeLang))
	}

	return buf.String()
}

// processLine applies simple markdown formatting to a single line of input.
func processLine(line string) string {
	line = boldRegex.ReplaceAllString(line, "\033[1m$1\033[0m")
	line = italicRegex.ReplaceAllString(line, "\033[3m$1\033[0m")
	line = inlineCodeRegex.ReplaceAllString(line, "\033[32m$1\033[0m")

	if strings.HasPrefix(line, "# ") {
		return "\033[1;36m" + line[2:] + "\033[0m"
	}
	if strings.HasPrefix(line, "## ") {
		return "\033[1;35m" + line[3:] + "\033[0m"
	}
	if strings.HasPrefix(line, "### ") {
		return "\033[1;34m" + line[4:] + "\033[0m"
	}
	if strings.HasPrefix(line, "#### ") {
		return "\033[1;33m" + line[5:] + "\033[0m"
	}

	if strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ") {
		return "  \033[34m*\033[0m " + line[2:]
	}

	if numberedListRegex.MatchString(line) {
		line = numberedListRegex.ReplaceAllString(line, "  \033[34m$1.\033[0m ")
	}

	return linkRegex.ReplaceAllString(line, "\033[4m$1\033[0m")
}

// highlightCode adds ANSI colors to code blocks based on the declared language.
func highlightCode(code, lang string) string {
	var buf bytes.Buffer

	colors := map[string]string{
		"go":         "\033[94m",
		"python":     "\033[96m",
		"js":         "\033[93m",
		"javascript": "\033[93m",
		"ts":         "\033[93m",
		"typescript": "\033[93m",
		"bash":       "\033[92m",
		"sh":         "\033[92m",
		"yaml":       "\033[33m",
		"yml":        "\033[33m",
		"json":       "\033[35m",
		"sql":        "\033[36m",
	}

	color := colors[strings.ToLower(lang)]
	if color == "" {
		color = "\033[37m"
	}

	reset := "\033[0m"
	lines := strings.Split(code, "\n")
	for i, line := range lines {
		line = highlightStrings(line)
		line = highlightComments(line)
		line = highlightNumbers(line)

		buf.WriteString(color + line + reset)
		if i < len(lines)-1 {
			buf.WriteString("\r\n")
		}
	}

	return buf.String()
}

// highlightStrings colors double-quoted string literals.
func highlightStrings(line string) string {
	return stringRegex.ReplaceAllStringFunc(line, func(s string) string {
		return "\033[33m" + s + "\033[0m"
	})
}

// highlightComments colors whole-line comments for the supported lightweight lexers.
func highlightComments(line string) string {
	trimmed := strings.TrimSpace(line)
	if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "#") {
		return "\033[90m" + line + "\033[0m"
	}

	return line
}

// highlightNumbers colors decimal integer literals.
func highlightNumbers(line string) string {
	return numberRegex.ReplaceAllString(line, "\033[31m$0\033[0m")
}

// resolveRuntimePath returns an environment override when present, otherwise the fallback path.
func resolveRuntimePath(envKey, fallback string) string {
	if value, ok := os.LookupEnv(envKey); ok {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}

	return fallback
}

// resolveConfigRelativePath anchors relative config-defined paths to the config file directory.
func resolveConfigRelativePath(configPath, candidate string) string {
	if candidate == "" || filepath.IsAbs(candidate) {
		return candidate
	}

	baseDir := filepath.Dir(configPath)
	if baseDir == "." || baseDir == "" {
		return candidate
	}

	return filepath.Join(baseDir, candidate)
}

// writeString writes a string to an SSH channel and returns the write error, if any.
func writeString(writer io.Writer, value string) error {
	_, err := io.WriteString(writer, value)
	return err
}

// logField escapes control characters before attacker-controlled values are written to logs.
func logField(value string) string {
	return strconv.QuoteToASCII(value)
}
