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

package gitops

// Annotations written by the operator on managed GitOps objects.
const (
	// AnnotationManaged marks an object as currently managed by the freeze-operator.
	AnnotationManaged = "freeze-operator.io/managed"

	// AnnotationManagedByPolicy records which policy (name) put the object in pause.
	AnnotationManagedByPolicy = "freeze-operator.io/managed-by-policy"

	// AnnotationOriginalAutoSync stores the original ArgoCD spec.syncPolicy.automated JSON
	// before the operator disabled it. Value is the raw JSON object, or "null" if autosync
	// was already disabled when the operator touched it.
	AnnotationOriginalAutoSync = "freeze-operator.io/original-autosync"

	// AnnotationOriginalSuspend stores the original Flux spec.suspend boolean value ("true"/"false")
	// before the operator set it to true.
	AnnotationOriginalSuspend = "freeze-operator.io/original-suspend"

	// annotationManagedValue is the value written to AnnotationManaged.
	annotationManagedValue = "true"
)
