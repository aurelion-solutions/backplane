// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package siem

// Zabbix is a placeholder for a Zabbix sender-protocol sink.
type Zabbix struct{ Stub }

// RegisterZabbix wires the "zabbix" stub provider.
func RegisterZabbix(f *Factory) {
	f.Register("zabbix", func() (Sink, error) { return Zabbix{}, nil })
}
