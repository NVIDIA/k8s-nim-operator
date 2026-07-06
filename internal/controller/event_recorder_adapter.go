/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/events"
	"k8s.io/client-go/tools/record"
)

type legacyEventRecorderAdapter struct {
	recorder events.EventRecorder
}

var _ record.EventRecorder = legacyEventRecorderAdapter{}

func newLegacyEventRecorder(recorder events.EventRecorder) record.EventRecorder {
	return legacyEventRecorderAdapter{recorder: recorder}
}

func (r legacyEventRecorderAdapter) Event(object runtime.Object, eventtype, reason, message string) {
	r.recorder.Eventf(object, nil, eventtype, reason, reason, "%s", message)
}

func (r legacyEventRecorderAdapter) Eventf(object runtime.Object, eventtype, reason, messageFmt string, args ...interface{}) {
	r.recorder.Eventf(object, nil, eventtype, reason, reason, messageFmt, args...)
}

func (r legacyEventRecorderAdapter) AnnotatedEventf(object runtime.Object, _ map[string]string, eventtype, reason, messageFmt string, args ...interface{}) {
	r.Eventf(object, eventtype, reason, messageFmt, args...)
}
