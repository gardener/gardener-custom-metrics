// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package input_data_registry

type mockWatcher struct {
	EventTypes []KapiEventType
	EventKapis []ShootKapi
	Watcher    KapiWatcher
}

func newMockWatcher() *mockWatcher {
	mock := &mockWatcher{
		EventTypes: make([]KapiEventType, 0),
		EventKapis: make([]ShootKapi, 0),
	}
	mock.Watcher = func(kapi ShootKapi, event KapiEventType) {
		mock.EventTypes = append(mock.EventTypes, event)
		mock.EventKapis = append(mock.EventKapis, kapi)
	}
	return mock
}
