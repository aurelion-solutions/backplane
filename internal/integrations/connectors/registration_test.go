// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package connectors

import (
	"strings"
	"testing"
)

func TestRegistrationMessage_Validate(t *testing.T) {
	cases := []struct {
		name string
		msg  RegistrationMessage
		want string
	}{
		{"unknown event_type", RegistrationMessage{EventType: "garbage", InstanceID: "i"}, "unknown event_type"},
		{"empty instance_id", RegistrationMessage{EventType: EventTypeRegistered, InstanceID: " "}, "instance_id must be non-empty"},
		{"oversize instance_id", RegistrationMessage{EventType: EventTypeRegistered, InstanceID: strings.Repeat("x", 256)}, "at most 255 characters"},
		{"empty tag", RegistrationMessage{EventType: EventTypeHeartbeat, InstanceID: "ok", Tags: []string{"a", " "}}, "tags must not contain empty entries"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.msg.Validate()
			if err == nil {
				t.Fatalf("expected error containing %q", tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q must contain %q", err, tc.want)
			}
		})
	}
}

func TestRegistrationMessage_Validate_OK(t *testing.T) {
	msg := RegistrationMessage{
		EventType:  EventTypeRegistered,
		InstanceID: "sf-prod-1",
		Tags:       []string{"salesforce", "prod"},
	}
	if err := msg.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
