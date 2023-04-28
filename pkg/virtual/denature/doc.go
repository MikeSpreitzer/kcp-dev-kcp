/*
Copyright 2023 The KCP Authors.

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

// This package and its sub-packages provides a view that denatures (renders inert)
// all objects that kcp normally gives some interpretation to, except those listed in
// https://docs.kcp-edge.io/docs/coding-milestones/poc2023q1/outline/#needs-to-be-natured-in-center-and-edge
//
// This allows a client to use this view for storage of objects without any of the usual consequences
// (excepting the desired consequence of making it possible to store certain other objects).
// The denaturing is done by translating API group <some.thing> to <some.thing>.denatured and "" to "denatured".
//
// For example, a request for
// GET /services/denature/clusters/<clustername>/apis/<group>/<version>/<remainder>
// becomes a request for
// GET /clusters/<clustername>/apis/<group,denatured>/<version>/<remainder>
// with the request body undergoing denaturing and the response body undergoing the inverse.
// Yeah, list and watch too.
package denature

const VirtualWorkspaceName string = "denature"
