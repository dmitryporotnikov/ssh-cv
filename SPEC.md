# SSH CV Viewer - Specification

## Project Overview
- **Project name**: ssh-cv
- **Type**: Secure SSH kiosk server
- **Core functionality**: SSH server that authenticates any user then displays a formatted markdown CV file in the terminal with hacker-style animations
- **Target users**: IT professionals showcasing their CV via SSH

## Functionality Specification

### Core Features
1. **SSH Server**
   - Listen on configurable port (default: 2222, configurable via `config.yaml`)
   - Accept any username/password/key combination
   - Support all common encryption algorithms
   - Handle multiple concurrent connections

2. **Authentication**
   - Permissive authentication: accept any credential (no auth required)
   - Support password authentication
   - Support public key authentication (accept any key)

3. **Session Behavior**
   - After successful auth, display hacker-style boot animation
   - Show formatted markdown content from `info.md`
   - Provide clean exit option
   - Auto-disconnect after client disconnect or timeout

4. **Security Restrictions**
   - NO shell access
   - NO command execution
   - NO SFTP access
   - NO port forwarding
   - NO interactive shell - only output display

### User Interactions
1. User connects via SSH
2. Server authenticates successfully (no credentials needed)
3. Server displays hacker boot animation sequence
4. Server displays formatted markdown CV
5. User reads CV
6. User types 'q', 'x', or Ctrl+C, or closes connection
7. Connection terminates cleanly with goodbye message

### Data Handling
- Read CV from `info.md` file in same directory as server
- Read configuration from `config.yaml` (optional, defaults applied if missing)
- Parse and format markdown for terminal display
- Built-in syntax highlighting for code blocks (no pygments needed)

### Edge Cases
- Missing `info.md` file: show error message and exit
- Empty `info.md` file: show message that no content is available
- Missing `config.yaml` file: use default configuration
- Client disconnect: clean up resources

## Configuration File

The server reads configuration from `config.yaml` in the same directory. All settings are optional and have sensible defaults.

### Configuration Options

```yaml
server:
  # Port to listen on
  port: 2222                    # Default: 2222

animations:
  # Matrix-style lines displayed during boot sequence
  # Each string is displayed on a separate line
  matrix_lines:
    - "01001110 10101010 BINARY_INTRUSION_DETECTED"
    - "SYSTEM_HACK_V.3.7.1"
    - "NEURAL_LINK_ESTABLISHED"
    # Add more lines as desired

  # Delay between matrix lines (milliseconds)
  matrix_speed_ms: 60           # Default: 60

  # Number of progress scan lines during boot
  scan_lines_count: 3           # Default: 3

  # Speed of scan line animation (milliseconds per character)
  scan_speed_ms: 5              # Default: 5

  # Enable colorful glitch effect on ASCII art banner
  ascii_glitch: true            # Default: true

  # Delay between boot status messages (milliseconds)
  boot_delay_ms: 100            # Default: 100

  # Speed of typewriter effect for boot messages (ms per char)
  typewriter_speed_ms: 10       # Default: 10

  # Show the scanning progress lines during boot
  show_scan_lines: true         # Default: true

messages:
  # Shown in the access granted box
  access_granted: ">> ACCESS GRANTED <<"

  # Prompt shown after CV content
  press_to_exit: "[Press 'q' to disconnect]"

  # Shown when connection is terminated
  goodbye: "Connection terminated."

  # Shown after goodbye message
  hack_message: "HACK THE PLANET!"
```

### Example: Minimal Configuration (All Defaults)

```yaml
# Minimal config - everything uses defaults
```

### Example: Custom Theme

```yaml
server:
  port: 2222

animations:
  matrix_lines:
    - "CUSTOM_STRING_1"
    - "CUSTOM_STRING_2"
    ascii_glitch: false
    show_scan_lines: false

messages:
  access_granted: ">> WELCOME <<"
  press_to_exit: "[Press ENTER to continue]"
  goodbye: "Session ended."
  hack_message: "See you next time!"
```

## Acceptance Criteria
- [x] SSH server starts without errors
- [x] Any username/password combination authenticates successfully
- [x] Any public key is accepted
- [x] After login, hacker boot animation plays
- [x] After animation, markdown content is displayed formatted
- [x] User cannot execute any commands
- [x] User can type 'q' to exit cleanly
- [x] Connection terminates cleanly on exit
- [x] No shell access is possible
- [x] Code blocks in markdown are syntax-highlighted
- [x] Configuration file allows customization of animations and messages
- [x] Configuration file allows changing the listening port

## Technical Stack
- Server written in Go
- Uses `golang.org/x/crypto/ssh` for SSH protocol
- Uses `gopkg.in/yaml.v3` for configuration parsing
- No external dependencies for markdown rendering or syntax highlighting
