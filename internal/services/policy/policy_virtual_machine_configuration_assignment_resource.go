package policy

import (
	"fmt"
	"log"
	"time"

	"github.com/Azure/azure-sdk-for-go/services/guestconfiguration/mgmt/2020-06-25/guestconfiguration"
	"github.com/hashicorp/terraform-provider-azurerm/helpers/azure"
	"github.com/hashicorp/terraform-provider-azurerm/helpers/tf"
	"github.com/hashicorp/terraform-provider-azurerm/internal/clients"
	"github.com/hashicorp/terraform-provider-azurerm/internal/location"
	computeParse "github.com/hashicorp/terraform-provider-azurerm/internal/services/compute/parse"
	computeValidate "github.com/hashicorp/terraform-provider-azurerm/internal/services/compute/validate"
	"github.com/hashicorp/terraform-provider-azurerm/internal/services/policy/parse"
	"github.com/hashicorp/terraform-provider-azurerm/internal/tf/pluginsdk"
	"github.com/hashicorp/terraform-provider-azurerm/internal/tf/validation"
	"github.com/hashicorp/terraform-provider-azurerm/internal/timeouts"
	"github.com/hashicorp/terraform-provider-azurerm/utils"
)

func resourcePolicyVirtualMachineConfigurationAssignment() *pluginsdk.Resource {
	return &pluginsdk.Resource{
		Create: resourcePolicyVirtualMachineConfigurationAssignmentCreateUpdate,
		Read:   resourcePolicyVirtualMachineConfigurationAssignmentRead,
		Update: resourcePolicyVirtualMachineConfigurationAssignmentCreateUpdate,
		Delete: resourcePolicyVirtualMachineConfigurationAssignmentDelete,

		Timeouts: &pluginsdk.ResourceTimeout{
			Create: pluginsdk.DefaultTimeout(30 * time.Minute),
			Read:   pluginsdk.DefaultTimeout(5 * time.Minute),
			Update: pluginsdk.DefaultTimeout(30 * time.Minute),
			Delete: pluginsdk.DefaultTimeout(30 * time.Minute),
		},

		Importer: pluginsdk.ImporterValidatingResourceId(func(id string) error {
			_, err := parse.VirtualMachineConfigurationAssignmentID(id)
			return err
		}),

		Schema: map[string]*pluginsdk.Schema{
			"name": {
				Type:     pluginsdk.TypeString,
				Required: true,
				ForceNew: true,
			},

			"location": azure.SchemaLocation(),

			"virtual_machine_id": {
				Type:         pluginsdk.TypeString,
				Required:     true,
				ForceNew:     true,
				ValidateFunc: computeValidate.VirtualMachineID,
			},

			"configuration": {
				Type:     pluginsdk.TypeList,
				Required: true,
				MaxItems: 1,
				Elem: &pluginsdk.Resource{
					Schema: map[string]*pluginsdk.Schema{
						"name": {
							Type:         pluginsdk.TypeString,
							Required:     true,
							ValidateFunc: validation.StringIsNotEmpty,
						},

						"assignment_type": {
							Type:     pluginsdk.TypeString,
							Optional: true,
							ValidateFunc: validation.StringInSlice([]string{
								string(guestconfiguration.AssignmentTypeAudit),
								string(guestconfiguration.AssignmentTypeDeployAndAutoCorrect),
								string(guestconfiguration.AssignmentTypeApplyAndAutoCorrect),
								string(guestconfiguration.AssignmentTypeApplyAndMonitor),
							}, false),
						},

						"content_hash": {
							Type:         pluginsdk.TypeString,
							Optional:     true,
							ValidateFunc: validation.StringIsNotEmpty,
						},

						"content_uri": {
							Type:         pluginsdk.TypeString,
							Optional:     true,
							ValidateFunc: validation.IsURLWithScheme([]string{"http", "https"}),
						},

						"parameter": {
							Type:     pluginsdk.TypeSet,
							Optional: true,
							Elem: &pluginsdk.Resource{
								Schema: map[string]*pluginsdk.Schema{
									"name": {
										Type:         pluginsdk.TypeString,
										Required:     true,
										ValidateFunc: validation.StringIsNotEmpty,
									},

									"value": {
										Type:     pluginsdk.TypeString,
										Required: true,
									},
								},
							},
						},

						"version": {
							Type:     pluginsdk.TypeString,
							Optional: true,
						},
					},
				},
			},
		},
	}
}

func resourcePolicyVirtualMachineConfigurationAssignmentCreateUpdate(d *pluginsdk.ResourceData, meta interface{}) error {
	subscriptionId := meta.(*clients.Client).Account.SubscriptionId
	client := meta.(*clients.Client).Policy.GuestConfigurationAssignmentsClient
	ctx, cancel := timeouts.ForCreateUpdate(meta.(*clients.Client).StopContext, d)
	defer cancel()

	vmId, err := computeParse.VirtualMachineID(d.Get("virtual_machine_id").(string))
	if err != nil {
		return err
	}

	id := parse.NewVirtualMachineConfigurationAssignmentID(subscriptionId, vmId.ResourceGroup, vmId.Name, d.Get("name").(string))

	if d.IsNewResource() {
		existing, err := client.Get(ctx, id.ResourceGroup, id.GuestConfigurationAssignmentName, id.VirtualMachineName)
		if err != nil {
			if !utils.ResponseWasNotFound(existing.Response) {
				return fmt.Errorf("checking for present of existing %s: %+v", id, err)
			}
		}
		if !utils.ResponseWasNotFound(existing.Response) {
			return tf.ImportAsExistsError("azurerm_policy_virtual_machine_configuration_assignment", id.ID())
		}
	}

	parameter := guestconfiguration.Assignment{
		Name:     utils.String(d.Get("name").(string)),
		Location: utils.String(location.Normalize(d.Get("location").(string))),
		Properties: &guestconfiguration.AssignmentProperties{
			GuestConfiguration: expandGuestConfigurationAssignment(d.Get("configuration").([]interface{})),
		},
	}
	if _, err := client.CreateOrUpdate(ctx, id.GuestConfigurationAssignmentName, parameter, id.ResourceGroup, id.VirtualMachineName); err != nil {
		return fmt.Errorf("creating/updating %s: %+v", id, err)
	}

	d.SetId(id.ID())

	return resourcePolicyVirtualMachineConfigurationAssignmentRead(d, meta)
}

func resourcePolicyVirtualMachineConfigurationAssignmentRead(d *pluginsdk.ResourceData, meta interface{}) error {
	subscriptionId := meta.(*clients.Client).Account.SubscriptionId
	client := meta.(*clients.Client).Policy.GuestConfigurationAssignmentsClient
	ctx, cancel := timeouts.ForRead(meta.(*clients.Client).StopContext, d)
	defer cancel()

	id, err := parse.VirtualMachineConfigurationAssignmentID(d.Id())
	if err != nil {
		return err
	}

	resp, err := client.Get(ctx, id.ResourceGroup, id.GuestConfigurationAssignmentName, id.VirtualMachineName)
	if err != nil {
		if utils.ResponseWasNotFound(resp.Response) {
			log.Printf("[INFO] guestConfiguration %q does not exist - removing from state", d.Id())
			d.SetId("")
			return nil
		}
		return fmt.Errorf("retrieving %s: %+v", id, err)
	}

	vmId := computeParse.NewVirtualMachineID(subscriptionId, id.ResourceGroup, id.VirtualMachineName)
	d.Set("name", id.GuestConfigurationAssignmentName)
	d.Set("virtual_machine_id", vmId.ID())
	d.Set("location", location.NormalizeNilable(resp.Location))

	if props := resp.Properties; props != nil {
		if err := d.Set("configuration", flattenGuestConfigurationAssignment(props.GuestConfiguration)); err != nil {
			return fmt.Errorf("setting `configuration`: %+v", err)
		}
	}
	return nil
}

func resourcePolicyVirtualMachineConfigurationAssignmentDelete(d *pluginsdk.ResourceData, meta interface{}) error {
	client := meta.(*clients.Client).Policy.GuestConfigurationAssignmentsClient
	ctx, cancel := timeouts.ForDelete(meta.(*clients.Client).StopContext, d)
	defer cancel()

	id, err := parse.VirtualMachineConfigurationAssignmentID(d.Id())
	if err != nil {
		return err
	}

	if _, err := client.Delete(ctx, id.ResourceGroup, id.GuestConfigurationAssignmentName, id.VirtualMachineName); err != nil {
		return fmt.Errorf("deleting %s: %+v", id, err)
	}

	return nil
}

func expandGuestConfigurationAssignment(input []interface{}) *guestconfiguration.Navigation {
	if len(input) == 0 {
		return nil
	}
	v := input[0].(map[string]interface{})

	result := guestconfiguration.Navigation{
		Name:                   utils.String(v["name"].(string)),
		Version:                utils.String(v["version"].(string)),
		ConfigurationParameter: expandGuestConfigurationAssignmentConfigurationParameters(v["parameter"].(*pluginsdk.Set).List()),
	}

	if v, ok := v["assignment_type"]; ok {
		result.AssignmentType = guestconfiguration.AssignmentType(v.(string))
	}

	if v, ok := v["content_hash"]; ok {
		result.ContentHash = utils.String(v.(string))
	}

	if v, ok := v["content_uri"]; ok {
		result.ContentURI = utils.String(v.(string))
	}

	return &result
}

func expandGuestConfigurationAssignmentConfigurationParameters(input []interface{}) *[]guestconfiguration.ConfigurationParameter {
	results := make([]guestconfiguration.ConfigurationParameter, 0)
	for _, item := range input {
		v := item.(map[string]interface{})
		results = append(results, guestconfiguration.ConfigurationParameter{
			Name:  utils.String(v["name"].(string)),
			Value: utils.String(v["value"].(string)),
		})
	}
	return &results
}

func flattenGuestConfigurationAssignment(input *guestconfiguration.Navigation) []interface{} {
	if input == nil {
		return make([]interface{}, 0)
	}

	var name string
	if input.Name != nil {
		name = *input.Name
	}
	var version string
	if input.Version != nil {
		version = *input.Version
	}
	var assignmentType guestconfiguration.AssignmentType
	if input.AssignmentType != "" {
		assignmentType = input.AssignmentType
	}
	var contentHash string
	if input.ContentHash != nil {
		contentHash = *input.ContentHash
	}
	var contentUri string
	if input.ContentURI != nil {
		contentUri = *input.ContentURI
	}
	return []interface{}{
		map[string]interface{}{
			"name":            name,
			"assignment_type": string(assignmentType),
			"content_hash":    contentHash,
			"content_uri":     contentUri,
			"parameter":       flattenGuestConfigurationAssignmentConfigurationParameters(input.ConfigurationParameter),
			"version":         version,
		},
	}
}

func flattenGuestConfigurationAssignmentConfigurationParameters(input *[]guestconfiguration.ConfigurationParameter) []interface{} {
	results := make([]interface{}, 0)
	if input == nil {
		return results
	}

	for _, item := range *input {
		var name string
		if item.Name != nil {
			name = *item.Name
		}
		var value string
		if item.Value != nil {
			value = *item.Value
		}
		results = append(results, map[string]interface{}{
			"name":  name,
			"value": value,
		})
	}
	return results
}
