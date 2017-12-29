/*
Copyright 2017 The Kubernetes Authors.

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

package flexvolume

import (
	"context"
	"testing"
)

func TestDetach(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	plugin, _ := testPlugin()
	plugin.runner = fakeRunner(
		assertDriverCall(t, notSupportedOutput(), detachCmd,
			"sdx", "localhost"),
	)

	d, _ := plugin.NewDetacher()
	d.Detach(ctx, "sdx", "localhost")
}

func TestUnmountDevice(t *testing.T) {
	plugin, rootDir := testPlugin()
	plugin.runner = fakeRunner(
		assertDriverCall(t, notSupportedOutput(), unmountDeviceCmd,
			rootDir+"/mount-dir"),
	)

	d, _ := plugin.NewDetacher()
	d.UnmountDevice(rootDir + "/mount-dir")
}
