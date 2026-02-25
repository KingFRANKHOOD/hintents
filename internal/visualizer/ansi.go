// Copyright 2025 Erst Users
// SPDX-License-Identifier: Apache-2.0

package visualizer

import "internal/terminal"

// ANSI SGR escape codes used for terminal colour output.
// These are aliases for the constants in internal/terminal so that
// there is a single source of truth for the escape sequences.
const (
	sgrReset   = terminal.SGRReset
	sgrRed     = terminal.SGRRed
	sgrGreen   = terminal.SGRGreen
	sgrYellow  = terminal.SGRYellow
	sgrBlue    = terminal.SGRBlue
	sgrMagenta = terminal.SGRMagenta
	sgrCyan    = terminal.SGRCyan
	sgrDim     = terminal.SGRDim
	sgrBold    = terminal.SGRBold
)
