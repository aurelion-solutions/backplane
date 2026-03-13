// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package siem

// Rsyslog is a placeholder for an RFC-5424 syslog sink over UDP/TCP/TLS.
type Rsyslog struct{ Stub }

// RegisterRsyslog wires the "rsyslog" stub provider.
func RegisterRsyslog(f *Factory) {
	f.Register("rsyslog", func() (Sink, error) { return Rsyslog{}, nil })
}
