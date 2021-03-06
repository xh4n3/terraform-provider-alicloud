package alicloud

import (
	"time"

	"github.com/aliyun/alibaba-cloud-sdk-go/services/cbn"
	"github.com/hashicorp/terraform/helper/resource"
	"github.com/hashicorp/terraform/helper/schema"
	"github.com/terraform-providers/terraform-provider-alicloud/alicloud/connectivity"
)

func resourceAlicloudCenRouteEntry() *schema.Resource {
	return &schema.Resource{
		Create: resourceAlicloudCenRouteEntryCreate,
		Read:   resourceAlicloudCenRouteEntryRead,
		Delete: resourceAlicloudCenRouteEntryDelete,
		Importer: &schema.ResourceImporter{
			State: schema.ImportStatePassthrough,
		},

		Schema: map[string]*schema.Schema{
			"instance_id": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			"route_table_id": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			"cidr_block": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
		},
	}
}

func resourceAlicloudCenRouteEntryCreate(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*connectivity.AliyunClient)
	cenService := CenService{client}
	cenId := d.Get("instance_id").(string)
	vtbId := d.Get("route_table_id").(string)
	cidr := d.Get("cidr_block").(string)
	childInstanceId, childInstanceType, err := cenService.CreateCenRouteEntryParas(vtbId)
	if err != nil {
		return WrapError(err)
	}

	request := cbn.CreatePublishRouteEntriesRequest()
	request.CenId = cenId
	request.ChildInstanceId = childInstanceId
	request.ChildInstanceType = childInstanceType
	request.ChildInstanceRegionId = client.RegionId
	request.ChildInstanceRouteTableId = vtbId
	request.DestinationCidrBlock = cidr

	err = resource.Retry(3*time.Minute, func() *resource.RetryError {
		raw, err := client.WithCenClient(func(cbnClient *cbn.Client) (interface{}, error) {
			return cbnClient.PublishRouteEntries(request)
		})
		if err != nil {
			if IsExceptedErrors(err, []string{OperationBlocking, InvalidStateForOperationMsg}) {
				return resource.RetryableError(err)
			}
			return resource.NonRetryableError(err)
		}
		addDebug(request.GetActionName(), raw)
		return nil
	})
	if err != nil {
		return WrapErrorf(err, DefaultErrorMsg, "alicloud_cen_route_entry", request.GetActionName(), AlibabaCloudSdkGoERROR)
	}

	d.SetId(cenId + COLON_SEPARATED + vtbId + COLON_SEPARATED + cidr)

	err = cenService.WaitForCenRouterEntry(d.Id(), PUBLISHED, DefaultCenTimeout)
	if err != nil {
		return WrapError(err)
	}

	return resourceAlicloudCenRouteEntryRead(d, meta)
}

func resourceAlicloudCenRouteEntryRead(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*connectivity.AliyunClient)
	cenService := CenService{client}

	parts, err := ParseResourceId(d.Id(), 3)
	if err != nil {
		return WrapError(err)
	}
	cenId := parts[0]

	object, err := cenService.DescribeCenRouteEntry(d.Id())
	if err != nil {
		if NotFoundError(err) {
			d.SetId("")
			return nil
		}
		return WrapError(err)
	}

	if object.PublishStatus == string(NOPUBLISHED) {
		d.SetId("")
		return nil
	}

	d.Set("instance_id", cenId)
	d.Set("route_table_id", object.ChildInstanceRouteTableId)
	d.Set("cidr_block", object.DestinationCidrBlock)

	return nil
}

func resourceAlicloudCenRouteEntryDelete(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*connectivity.AliyunClient)
	cenService := CenService{client}

	cenId := d.Get("instance_id").(string)
	vtbId := d.Get("route_table_id").(string)
	cidr := d.Get("cidr_block").(string)
	childInstanceId, childInstanceType, err := cenService.CreateCenRouteEntryParas(vtbId)
	if err != nil {
		if NotFoundError(err) {
			return nil
		}
		return WrapError(err)
	}

	request := cbn.CreateWithdrawPublishedRouteEntriesRequest()
	request.CenId = cenId
	request.ChildInstanceId = childInstanceId
	request.ChildInstanceType = childInstanceType
	request.ChildInstanceRegionId = client.RegionId
	request.ChildInstanceRouteTableId = vtbId
	request.DestinationCidrBlock = cidr

	err = resource.Retry(5*time.Minute, func() *resource.RetryError {
		raw, err := client.WithCenClient(func(cbnClient *cbn.Client) (interface{}, error) {
			return cbnClient.WithdrawPublishedRouteEntries(request)
		})
		if err != nil {
			if IsExceptedErrors(err, []string{InvalidCenInstanceStatus, InternalError}) {
				return resource.RetryableError(err)
			}

			return resource.NonRetryableError(err)
		}
		addDebug(request.GetActionName(), raw)
		return nil
	})
	if err != nil {
		if IsExceptedErrors(err, []string{NotFoundRoute, InstanceNotExistMsg}) {
			return nil
		}
		return WrapErrorf(err, DataDefaultErrorMsg, d.Id(), request.GetActionName(), AlibabaCloudSdkGoERROR)

	}
	return WrapError(cenService.WaitForCenRouterEntry(d.Id(), Deleted, DefaultCenTimeoutLong))
}
