// Copyright 2019 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package scorecard

import (
	"context"
	"fmt"
	"time"

	"github.com/briandowns/spinner"
	"github.com/pkg/errors"

	asset "cloud.google.com/go/asset/apiv1"
	assetpb "google.golang.org/genproto/googleapis/cloud/asset/v1"
)

// inventoryConfig manages a CAI inventory
type inventoryConfig struct {
	projectID        string
	controlProjectID string
	organizationID   string
	gcsBucket        string
}

// Option for NewInventory
type Option func(*inventoryConfig)

// ControlProject sets the project for storing inventory data
func ControlProject(projectID string) Option {
	return func(inventory *inventoryConfig) {
		inventory.controlProjectID = projectID
	}
}

// TargetProject sets the project for storing inventory data
func TargetProject(projectID string) Option {
	return func(inventory *inventoryConfig) {
		inventory.projectID = projectID
	}
}

// NewInventory creates a new CAI inventory manager
func NewInventory(projectID string, bucketName string, options ...Option) (*inventoryConfig, error) {
	inventory := new(inventoryConfig)
	inventory.controlProjectID = projectID
	inventory.gcsBucket = bucketName

	for _, option := range options {
		option(inventory)
	}

	Log.Debug("Initializing inventory", "target", getParent(inventory), "control", inventory.controlProjectID)
	return inventory, nil
}

func getParent(inventory *inventoryConfig) string {
	if inventory.organizationID != "" {
		return fmt.Sprintf("organizations/%v", inventory.organizationID)
	}
	return fmt.Sprintf("projects/%v", inventory.projectID)
}

var destinationObjectNames = map[assetpb.ContentType]string{
	assetpb.ContentType_RESOURCE:   "resource_inventory.json",
	assetpb.ContentType_IAM_POLICY: "iam_inventory.json",
}

func (inventory inventoryConfig) getGcsDestination(contentType assetpb.ContentType) *assetpb.GcsDestination_Uri {
	objectName := destinationObjectNames[contentType]
	return &assetpb.GcsDestination_Uri{
		Uri: fmt.Sprintf("gs://%v/%v", inventory.gcsBucket, objectName),
	}
}

func exportInventoryToGcs(inventory *inventoryConfig, contentType assetpb.ContentType) error {
	ctx := context.Background()
	c, err := asset.NewClient(ctx)
	if err != nil {
		return err
	}

	destination := inventory.getGcsDestination(contentType)
	req := &assetpb.ExportAssetsRequest{
		Parent:      getParent(inventory),
		ContentType: contentType,
		OutputConfig: &assetpb.OutputConfig{
			Destination: &assetpb.OutputConfig_GcsDestination{
				GcsDestination: &assetpb.GcsDestination{
					ObjectUri: destination,
				},
			},
		},
	}

	op, err := c.ExportAssets(ctx, req)
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("destination = %v", destination))
	}

	_, err = op.Wait(ctx)
	return err
}

// ExportInventory creates a new inventory export
func ExportInventory(inventory *inventoryConfig) error {
	s := spinner.New(spinner.CharSets[9], 100*time.Millisecond)
	s.Prefix = "Exporting Cloud Asset Inventory to GCS bucket... "
	s.Start()
	err := exportInventoryToGcs(inventory, assetpb.ContentType_RESOURCE)
	err = exportInventoryToGcs(inventory, assetpb.ContentType_IAM_POLICY)
	s.Stop()

	if err != nil {
		return err
	}

	return nil
}
