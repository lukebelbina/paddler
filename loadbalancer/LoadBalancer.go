package loadbalancer

import (
	"errors"
	"net/http"
	"net/url"

	"github.com/distantmagic/paddler/goroutine"
	"github.com/distantmagic/paddler/llamacpp"
	"github.com/hashicorp/go-hclog"
)

var (
	ErrorNoTargetsAvailable = errors.New("no targets available")
)

type LoadBalancer struct {
	HttpClient                   *http.Client
	LoadBalancerTargetCollection *LoadBalancerTargetCollection
	Logger                       hclog.Logger
}

func (self *LoadBalancer) Balance(request *LoadBalancerRequest) (*url.URL, error) {
	headTarget := self.GetLlamaCppTargetForRequest(request)

	if headTarget == nil {
		return nil, ErrorNoTargetsAvailable
	}

	targetUrl := headTarget.
		LlamaCppClient.
		LlamaCppConfiguration.
		HttpAddress.
		BuildUrlWithPath("")

	self.Logger.Debug(
		"balancing",
		"target", targetUrl,
		"slots", headTarget.LlamaCppHealthStatus.SlotsIdle,
	)

	return targetUrl, nil
}

func (self *LoadBalancer) GetLlamaCppTargetForRequest(request *LoadBalancerRequest) *LlamaCppTarget {
	if request.IsSlottable() {
		return self.
			LoadBalancerTargetCollection.
			GetTargetWithFreeSlotsForBalancing().
			LlamaCppTarget
	}

	return self.
		LoadBalancerTargetCollection.
		GetHeadTarget().
		LlamaCppTarget
}

func (self *LoadBalancer) GetStatus() *LoadBalancerStatus {
	return &LoadBalancerStatus{
		RegisteredTargets: self.LoadBalancerTargetCollection.Len(),
	}
}

func (self *LoadBalancer) RegisterOrUpdateTarget(
	serverEventsChannel chan goroutine.ResultMessage,
	targetConfiguration *LlamaCppTargetConfiguration,
	llamaCppHealthStatus *llamacpp.LlamaCppHealthStatus,
) {
	existingTarget := self.LoadBalancerTargetCollection.GetTargetByConfiguration(targetConfiguration)

	if existingTarget != nil {
		self.updateTarget(
			serverEventsChannel,
			targetConfiguration,
			llamaCppHealthStatus,
			existingTarget,
		)

		return
	}

	self.registerTarget(
		serverEventsChannel,
		targetConfiguration,
		llamaCppHealthStatus,
	)
}

func (self *LoadBalancer) registerTarget(
	serverEventsChannel chan goroutine.ResultMessage,
	targetConfiguration *LlamaCppTargetConfiguration,
	llamaCppHealthStatus *llamacpp.LlamaCppHealthStatus,
) {
	self.Logger.Debug(
		"registering target",
		"host", targetConfiguration.LlamaCppConfiguration.HttpAddress.GetHostWithPort(),
	)

	self.LoadBalancerTargetCollection.RegisterTarget(&LlamaCppTarget{
		LlamaCppClient: &llamacpp.LlamaCppClient{
			HttpClient:            self.HttpClient,
			LlamaCppConfiguration: targetConfiguration.LlamaCppConfiguration,
		},
		LlamaCppHealthStatus:        llamaCppHealthStatus,
		LlamaCppTargetConfiguration: targetConfiguration,
	})

	serverEventsChannel <- goroutine.ResultMessage{
		Comment: "registered target",
	}
}

func (self *LoadBalancer) updateTarget(
	serverEventsChannel chan goroutine.ResultMessage,
	targetConfiguration *LlamaCppTargetConfiguration,
	llamaCppHealthStatus *llamacpp.LlamaCppHealthStatus,
	existingTarget *LlamaCppTarget,
) {
	self.Logger.Debug(
		"updating target",
		"host", targetConfiguration.LlamaCppConfiguration.HttpAddress.GetHostWithPort(),
	)

	existingTarget.LlamaCppHealthStatus.SlotsIdle = llamaCppHealthStatus.SlotsIdle
	existingTarget.LlamaCppHealthStatus.SlotsProcessing = llamaCppHealthStatus.SlotsProcessing
	existingTarget.LlamaCppHealthStatus.Status = llamaCppHealthStatus.Status

	self.LoadBalancerTargetCollection.FixTargetOrder(existingTarget)

	serverEventsChannel <- goroutine.ResultMessage{
		Comment: "updated target",
	}
}
