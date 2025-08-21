package pwpin

import (
	"context"
	"encoding/json"
	"fmt"
	"iter"

	"github.com/facebookincubator/go-belt/tool/logger"
	"github.com/go-ng/xatomic"
	"github.com/xaionaro-go/observability"
	pwmonitor "github.com/xaionaro-go/pipewire-monitor-go"
	"github.com/xaionaro-go/xsync"
)

type Node struct {
	ID    int
	Info  *pwmonitor.EventInfo
	Ports map[int]*Port
}

func (n *Node) String() string {
	if n.Info != nil && n.Info.Props != nil && n.Info.Props.NodeDescription != nil {
		return fmt.Sprintf("%d (%s)", n.ID, *n.Info.Props.NodeDescription)
	}
	return fmt.Sprintf("%d", n.ID)
}

type Sink struct {
	*Node
	OutboundLinks map[PortKey]*Link
}

type Source struct {
	*Node
	InboundLinks map[PortKey]*Link
}

type PortKey struct {
	NodeID int
	PortID int
}

type LinkKey struct {
	From PortKey
	To   PortKey
}

type Link struct {
	Info *pwmonitor.EventInfo
}

type SimplePlumber struct {
	Config *Config

	ActiveDataLocker xsync.Mutex
	ActiveSinks      map[int]*Sink
	ActiveSources    map[int]*Source
	ActivePorts      map[int]*Port
	ActiveLinks      map[LinkKey]*Link
}

func New() *SimplePlumber {
	return &SimplePlumber{
		ActiveSinks:   map[int]*Sink{},
		ActiveSources: map[int]*Source{},
		ActivePorts:   map[int]*Port{},
		ActiveLinks:   map[LinkKey]*Link{},
	}
}

func (p *SimplePlumber) SetConfig(config Config) {
	xatomic.StorePointer(&p.Config, ptr(config))
}

func (p *SimplePlumber) GetConfig() *Config {
	return xatomic.LoadPointer(&p.Config)
}

func (p *SimplePlumber) eventFilter(ctx context.Context, event *pwmonitor.Event) (_ret bool) {
	logger.Tracef(ctx, "eventFilter(%s)", jsoninze(event))
	defer func() { logger.Tracef(ctx, "/eventFilter(%s): %v", jsoninze(event), _ret) }()

	if event.Info == nil {
		logger.Tracef(ctx, "eventFilter(%s): no Info in event", jsoninze(event))
		return false
	}

	switch event.Type {
	case pwmonitor.EventTypePipewireInterfacePort:
		return true
	case pwmonitor.EventTypePipewireInterfaceLink:
		return true
	case pwmonitor.EventTypePipewireInterfaceNode:
		return true
	}
	return false
}

func (p *SimplePlumber) ServeContext(ctx context.Context) error {
	eventsCh := make(chan []*pwmonitor.Event)
	errCh := make(chan error)
	observability.Go(ctx, func(ctx context.Context) {
		errCh <- pwmonitor.Monitor(ctx, eventsCh, func(e *pwmonitor.Event) bool {
			return p.eventFilter(ctx, e)
		})
	})
	for {
		var events []*pwmonitor.Event
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-errCh:
			if err != nil {
				return fmt.Errorf("error monitoring events: %w", err)
			}
			logger.Debugf(ctx, "monitoring events stopped")
			return nil
		case events = <-eventsCh:
		}
		for _, e := range events {
			if err := p.processEvent(ctx, e); err != nil {
				return fmt.Errorf("error processing event: %w", err)
			}
		}
	}
}

func (p *SimplePlumber) processEvent(
	ctx context.Context,
	e *pwmonitor.Event,
) (_err error) {
	logger.Tracef(ctx, "processEvent(%s)", jsoninze(e))
	defer func() { logger.Tracef(ctx, "/processEvent(%s), %v", jsoninze(e), _err) }()

	switch e.Type {
	case pwmonitor.EventTypePipewireInterfaceNode:
		if e.Info == nil || e.Info.Props == nil || e.Info.Props.MediaClass == nil {
			return nil // do not know what to do
		}
		switch *e.Info.Props.MediaClass {
		case pwmonitor.MediaClassAudioSink:
			if err := p.processEventNodeAudioSink(ctx, e); err != nil {
				return fmt.Errorf("error processing event: %w", err)
			}
		case pwmonitor.MediaClassAudioSource:
			if err := p.processEventNodeAudioSource(ctx, e); err != nil {
				return fmt.Errorf("error processing event: %w", err)
			}
		case pwmonitor.MediaClassStreamOutputAudio:
			if err := p.processEventNodeStreamOutputAudio(ctx, e); err != nil {
				return fmt.Errorf("error processing event: %w", err)
			}
		case pwmonitor.MediaClassStreamInputAudio:
			if err := p.processEventNodeStreamInputAudio(ctx, e); err != nil {
				return fmt.Errorf("error processing event: %w", err)
			}
		}
	case pwmonitor.EventTypePipewireInterfacePort:
		if err := p.processEventPort(ctx, e); err != nil {
			return fmt.Errorf("error processing port event: %w", err)
		}
	case pwmonitor.EventTypePipewireInterfaceLink:
		if err := p.processEventLink(ctx, e); err != nil {
			return fmt.Errorf("error processing link event: %w", err)
		}
	default:
		return fmt.Errorf("unknown event type: %s", e.Type)
	}

	return nil
}

func (p *SimplePlumber) processEventLink(
	ctx context.Context,
	e *pwmonitor.Event,
) (_err error) {
	logger.Debugf(ctx, "processEventLink(%s)", jsoninze(e))
	defer func() { logger.Tracef(ctx, "/processEventLink(%s)", jsoninze(e)) }()
	return xsync.DoA2R1(ctx, &p.ActiveDataLocker, p.processEventLinkNoLock, ctx, e)
}

func jsoninze(obj any) string {
	data, _ := json.Marshal(obj)
	return string(data)
}

func (p *SimplePlumber) processEventLinkNoLock(
	ctx context.Context,
	e *pwmonitor.Event,
) (_err error) {
	logger.Tracef(ctx, "processEventLinkNoLock(%s)", jsoninze(e))
	defer func() { logger.Tracef(ctx, "/processEventLinkNoLock(%s)", jsoninze(e)) }()

	switch *e.Info.State {
	case pwmonitor.StateIdle:
		return p.forgetLink(ctx, e.Info)
	default:
		return p.addOrUpdateLink(ctx, e)
	}
}

func (p *SimplePlumber) forgetLink(ctx context.Context, eInfo *pwmonitor.EventInfo) error {
	logger.Debugf(ctx, "forgetLink(%s)", jsoninze(eInfo))
	defer func() { logger.Tracef(ctx, "/forgetLink(%s)", jsoninze(eInfo)) }()
	linkKey := LinkKey{
		From: PortKey{
			NodeID: *eInfo.Props.LinkOutputNode,
			PortID: *eInfo.Props.LinkOutputPort,
		},
		To: PortKey{
			NodeID: *eInfo.Props.LinkInputNode,
			PortID: *eInfo.Props.LinkInputPort,
		},
	}
	if _, ok := p.ActiveLinks[linkKey]; !ok {
		logger.Warnf(ctx, "forgetLink(%s): link not found", jsoninze(eInfo))
		return nil
	}
	if source := p.ActiveSources[linkKey.From.NodeID]; source != nil {
		delete(source.InboundLinks, linkKey.To)
	}
	if sink := p.ActiveSinks[linkKey.To.NodeID]; sink != nil {
		delete(sink.OutboundLinks, linkKey.From)
	}
	delete(p.ActiveLinks, linkKey)
	return nil
}

func (p *SimplePlumber) addOrUpdateLink(
	ctx context.Context,
	e *pwmonitor.Event,
) error {
	logger.Debugf(ctx, "addOrUpdateLink(%s)", jsoninze(e))
	defer func() { logger.Tracef(ctx, "/addOrUpdateLink(%s)", jsoninze(e)) }()

	linkKey := LinkKey{
		From: PortKey{
			NodeID: *e.Info.Props.LinkOutputNode,
			PortID: *e.Info.Props.LinkOutputPort,
		},
		To: PortKey{
			NodeID: *e.Info.Props.LinkInputNode,
			PortID: *e.Info.Props.LinkInputPort,
		},
	}

	if _, ok := p.ActiveLinks[linkKey]; ok {
		logger.Debugf(ctx, "processEventLink(%s): link already exists", jsoninze(e))
		return nil
	}

	nodeInput := p.ActiveSources[linkKey.From.NodeID]
	if nodeInput == nil {
		logger.Debugf(ctx, "processEventLink(%s): source node %d not found", jsoninze(e), linkKey.From.NodeID) // TODO: investigate why this happens
		return nil
	}
	nodeOutput := p.ActiveSinks[linkKey.To.NodeID]
	if nodeOutput == nil {
		logger.Debugf(ctx, "processEventLink(%s): sink node %d not found", jsoninze(e), linkKey.To.NodeID) // TODO: investigate why this happens
		return nil
	}

	logger.Debugf(ctx, "processEventLink(%s): new link", jsoninze(e))
	link := &Link{Info: e.Info}
	p.ActiveLinks[linkKey] = link
	nodeInput.InboundLinks[linkKey.To] = link
	nodeOutput.OutboundLinks[linkKey.From] = link

	portInput := nodeInput.Node.Ports[linkKey.From.PortID]
	if portInput == nil {
		logger.Warnf(ctx, "processEventLink(%s): input port %d not found", jsoninze(e), linkKey.From.PortID)
		return nil
	}
	portOutput := nodeOutput.Node.Ports[linkKey.To.PortID]
	if portOutput == nil {
		logger.Warnf(ctx, "processEventLink(%s): output port %d not found", jsoninze(e), linkKey.To.PortID)
		return nil
	}

	err := p.fixConnections(ctx, nodeInput.Node, portInput)
	if err != nil {
		return fmt.Errorf("error making connections for the input node&port %d:%d: %w", linkKey.From.NodeID, linkKey.From.PortID, err)
	}

	err = p.fixConnections(ctx, nodeOutput.Node, portOutput)
	if err != nil {
		return fmt.Errorf("error making connections for the output node&port %d:%d: %w", linkKey.To.NodeID, linkKey.To.PortID, err)
	}

	return nil
}

func (p *SimplePlumber) processEventNodeAudioSink(ctx context.Context, e *pwmonitor.Event) error {
	logger.Debugf(ctx, "processEventNodeAudioSink(%s)", jsoninze(e))
	defer func() { logger.Tracef(ctx, "/processEventNodeAudioSink(%s)", jsoninze(e)) }()
	return xsync.DoA2R1(ctx, &p.ActiveDataLocker, p.processEventNodeAudioSinkNoLock, ctx, e)
}

func (p *SimplePlumber) processEventNodeAudioSinkNoLock(
	ctx context.Context,
	e *pwmonitor.Event,
) (_err error) {
	logger.Tracef(ctx, "processEventNodeAudioSinkNoLock(%s)", jsoninze(e))
	defer func() { logger.Tracef(ctx, "/processEventNodeAudioSinkNoLock(%s)", jsoninze(e)) }()
	switch *e.Info.State {
	case pwmonitor.StateIdle:
		return p.forgetSink(ctx, e.ID)
	default:
		return p.addOrUpdateSink(ctx, e)
	}
}

func (p *SimplePlumber) processEventNodeAudioSource(ctx context.Context, e *pwmonitor.Event) error {
	logger.Debugf(ctx, "processEventNodeAudioSource(%s)", jsoninze(e))
	defer func() { logger.Tracef(ctx, "/processEventNodeAudioSource(%s)", jsoninze(e)) }()
	return xsync.DoA2R1(ctx, &p.ActiveDataLocker, p.processEventNodeAudioSourceNoLock, ctx, e)
}

func (p *SimplePlumber) processEventNodeAudioSourceNoLock(
	ctx context.Context,
	e *pwmonitor.Event,
) (_err error) {
	logger.Tracef(ctx, "processEventNodeAudioSourceNoLock(%s)", jsoninze(e))
	defer func() { logger.Tracef(ctx, "/processEventNodeAudioSourceNoLock(%s)", jsoninze(e)) }()
	switch *e.Info.State {
	case pwmonitor.StateIdle:
		return p.forgetSource(ctx, e.ID)
	default:
		return p.addOrUpdateSource(ctx, e)
	}
}

func (p *SimplePlumber) processEventNodeStreamOutputAudio(ctx context.Context, e *pwmonitor.Event) error {
	logger.Debugf(ctx, "processEventNodeStreamOutputAudio(%s)", jsoninze(e))
	defer func() { logger.Tracef(ctx, "/processEventNodeStreamOutputAudio(%s)", jsoninze(e)) }()
	return xsync.DoA2R1(ctx, &p.ActiveDataLocker, p.processEventNodeStreamOutputAudioNoLock, ctx, e)
}

func (p *SimplePlumber) processEventNodeStreamOutputAudioNoLock(
	ctx context.Context,
	e *pwmonitor.Event,
) (_err error) {
	logger.Tracef(ctx, "processEventNodeStreamOutputAudioNoLock(%s)", jsoninze(e))
	defer func() { logger.Tracef(ctx, "/processEventNodeStreamOutputAudioNoLock(%s)", jsoninze(e)) }()
	switch *e.Info.State {
	case pwmonitor.StateIdle:
		return p.forgetSource(ctx, e.ID)
	default:
		return p.addOrUpdateSource(ctx, e)
	}
}

func (p *SimplePlumber) processEventNodeStreamInputAudio(ctx context.Context, e *pwmonitor.Event) error {
	logger.Debugf(ctx, "processEventNodeStreamInputAudio(%s): %s", jsoninze(e), *e.Info.State)
	defer func() { logger.Tracef(ctx, "/processEventNodeStreamInputAudio(%s): %s", jsoninze(e), *e.Info.State) }()
	return xsync.DoA2R1(ctx, &p.ActiveDataLocker, p.processEventNodeStreamInputAudioNoLock, ctx, e)
}

func (p *SimplePlumber) processEventNodeStreamInputAudioNoLock(
	ctx context.Context,
	e *pwmonitor.Event,
) (_err error) {
	logger.Tracef(ctx, "processEventNodeStreamInputAudioNoLock(%s)", jsoninze(e))
	defer func() { logger.Tracef(ctx, "/processEventNodeStreamInputAudioNoLock(%s)", jsoninze(e)) }()
	switch *e.Info.State {
	case pwmonitor.StateIdle:
		return p.forgetSink(ctx, e.ID)
	default:
		return p.addOrUpdateSink(ctx, e)
	}
}

func (p *SimplePlumber) addOrUpdateSink(ctx context.Context, e *pwmonitor.Event) error {
	logger.Tracef(ctx, "addOrUpdateSink(%s)", jsoninze(e))
	defer func() { logger.Tracef(ctx, "/addOrUpdateSink(%s)", jsoninze(e)) }()

	sink, ok := p.ActiveSinks[e.ID]
	if ok {
		logger.Debugf(ctx, "update sink: %s", jsoninze(e))
		sink.Info = e.Info
		return nil
	}

	logger.Debugf(ctx, "new sink: %s", jsoninze(e))
	sink = &Sink{
		Node: &Node{
			ID:    e.ID,
			Info:  e.Info,
			Ports: make(map[int]*Port),
		},
		OutboundLinks: make(map[PortKey]*Link),
	}
	p.ActiveSinks[e.ID] = sink
	return nil
}

func (p *SimplePlumber) addOrUpdateSource(ctx context.Context, e *pwmonitor.Event) error {
	logger.Tracef(ctx, "addOrUpdateSource(%s)", jsoninze(e))
	defer func() { logger.Tracef(ctx, "/addOrUpdateSource(%s)", jsoninze(e)) }()

	source, ok := p.ActiveSources[e.ID]
	if ok {
		logger.Debugf(ctx, "update source: %s", jsoninze(e))
		source.Info = e.Info
		return nil
	}

	logger.Debugf(ctx, "new source: %s", jsoninze(e))
	source = &Source{
		Node: &Node{
			ID:    e.ID,
			Info:  e.Info,
			Ports: make(map[int]*Port),
		},
		InboundLinks: make(map[PortKey]*Link),
	}
	p.ActiveSources[e.ID] = source
	return nil
}

func (p *SimplePlumber) processEventPort(ctx context.Context, e *pwmonitor.Event) error {
	logger.Debugf(ctx, "processEventPort(%s)", jsoninze(e))
	defer func() { logger.Tracef(ctx, "/processEventPort(%s)", jsoninze(e)) }()

	return xsync.DoA2R1(ctx, &p.ActiveDataLocker, p.processEventPortNoLock, ctx, e)
}

func (p *SimplePlumber) processEventPortNoLock(
	ctx context.Context,
	e *pwmonitor.Event,
) (_err error) {
	logger.Tracef(ctx, "processEventPortNoLock(%s)", jsoninze(e))
	defer func() { logger.Tracef(ctx, "/processEventPortNoLock(%s)", jsoninze(e)) }()

	switch {
	case e.Info == nil:
		return p.forgetPort(ctx, e.ID)
	default:
		return p.addOrUpdatePort(ctx, e)
	}
}

func (p *SimplePlumber) forgetPort(ctx context.Context, portID int) (_err error) {
	logger.Debugf(ctx, "forgetPort(%d)", portID)
	defer func() { logger.Tracef(ctx, "/forgetPort(%d)", portID, _err) }()

	port := p.ActivePorts[portID]
	if port == nil {
		logger.Warnf(ctx, "forgetPort(%d): port not found", portID)
		return nil
	}
	delete(p.ActivePorts, portID)

	nodeID := *port.Info.Props.NodeID
	var node *Node
	if sink, ok := p.ActiveSinks[nodeID]; ok {
		node = sink.Node
	} else if source, ok := p.ActiveSources[nodeID]; ok {
		node = source.Node
	}
	if node == nil {
		logger.Tracef(ctx, "forgetPort(%d): no node found for the port event", portID)
		return nil
	}

	delete(node.Ports, portID)
	return nil
}

func (p *SimplePlumber) addOrUpdatePort(ctx context.Context, e *pwmonitor.Event) error {
	logger.Debugf(ctx, "addOrUpdatePort(%s)", jsoninze(e))
	defer func() { logger.Tracef(ctx, "/addOrUpdatePort(%s)", jsoninze(e)) }()
	if e.Info == nil {
		return fmt.Errorf("no Info in event")
	}

	nodeID := *e.Info.Props.NodeID

	var node *Node
	if sink, ok := p.ActiveSinks[nodeID]; ok {
		node = sink.Node
	} else if source, ok := p.ActiveSources[nodeID]; ok {
		node = source.Node
	}
	if node == nil {
		logger.Tracef(ctx, "processEventPort(%s): no node found for the port event", jsoninze(e))
		return nil
	}

	port := &Port{
		ID:   e.ID,
		Info: e.Info,
	}
	node.Ports[e.ID] = port
	p.ActivePorts[e.ID] = port

	err := p.fixConnections(ctx, node, port)
	if err != nil {
		return fmt.Errorf("error making connections for port %d: %w", e.ID, err)
	}

	return nil
}

func (p *SimplePlumber) fixConnections(ctx context.Context, node *Node, port *Port) (_err error) {
	logger.Tracef(ctx, "fixConnections(node=%d, port=%d)", node.ID, port.ID)
	defer func() {
		logger.Tracef(ctx, "/fixConnections(node=%d, port=%d), %v", node.ID, port.ID, _err)
	}()

	cfg := p.GetConfig()
	if cfg == nil {
		return fmt.Errorf("no configuration set")
	}

	var myNodeSelectorFunc func(r *Route) Constraints
	var myPortSelectorFunc func(r *Route) Constraints
	var remoteNodeSelectorFunc func(r *Route) Constraints
	var remotePortSelectorFunc func(r *Route) Constraints
	var remoteNodes iter.Seq[*Node]
	if port.IsOutput() {
		myNodeSelectorFunc = func(r *Route) Constraints { return r.From.Node }
		myPortSelectorFunc = func(r *Route) Constraints { return r.From.Port }
		remoteNodeSelectorFunc = func(r *Route) Constraints { return r.To.Node }
		remotePortSelectorFunc = func(r *Route) Constraints { return r.To.Port }
		remoteNodes = func(yield func(*Node) bool) {
			for _, sink := range p.ActiveSinks {
				if !yield(sink.Node) {
					return
				}
			}
		}
	} else if port.IsInput() {
		myNodeSelectorFunc = func(r *Route) Constraints { return r.To.Node }
		myPortSelectorFunc = func(r *Route) Constraints { return r.To.Port }
		remoteNodeSelectorFunc = func(r *Route) Constraints { return r.From.Node }
		remotePortSelectorFunc = func(r *Route) Constraints { return r.From.Port }
		remoteNodes = func(yield func(*Node) bool) {
			for _, source := range p.ActiveSources {
				if !yield(source.Node) {
					return
				}
			}
		}
	} else {
		return fmt.Errorf("port %d is neither input nor output", port.ID)
	}

	alreadySet := map[PortKey]struct{}{}
	for _, route := range cfg.Routes {
		if !myNodeSelectorFunc(&route).Match(*node.Info.Props) {
			continue
		}
		if !myPortSelectorFunc(&route).Match(*port.Info.Props) {
			continue
		}

		if len(myNodeSelectorFunc(&route)) != 0 || len(myPortSelectorFunc(&route)) != 0 {
			logger.Debugf(ctx, "fixConnections: this port (%s: %s) matches the route rule %s, checking remote nodes and ports", jsoninze(*node.Info.Props), jsoninze(*port.Info.Props), jsoninze(route))
		}

		for remoteNode := range remoteNodes {
			if !remoteNodeSelectorFunc(&route).Match(*remoteNode.Info.Props) {
				continue
			}
			for _, remotePort := range remoteNode.Ports {
				remotePortKey := PortKey{
					NodeID: remoteNode.ID,
					PortID: remotePort.ID,
				}
				if _, ok := alreadySet[remotePortKey]; ok {
					continue
				}
				if !remotePortSelectorFunc(&route).Match(*remotePort.Info.Props) {
					continue
				}

				if node.ID == remoteNode.ID && port.ID == remotePort.ID {
					continue
				}

				alreadySet[remotePortKey] = struct{}{}
				if route.ShouldBeLinked == nil {
					continue
				}

				logger.Debugf(
					ctx,
					"fixConnections: making link state for node '%s', port '%s' to remote node '%s', port '%s' to %t due to route rule %s",
					node, port, remoteNode, remotePort,
					*route.ShouldBeLinked, route,
				)
				isChanged, err := p.makeLinkState(ctx, node.ID, port.ID, remoteNode.ID, remotePort.ID, *route.ShouldBeLinked)
				if err != nil {
					logger.Errorf(ctx, "error making link state for node '%s', port '%s' to remote node '%s', port '%s' to %t: %v",
						node, port, remoteNode, remotePort, *route.ShouldBeLinked, err)
					return nil
				}
				if isChanged {
					if *route.ShouldBeLinked {
						logger.Infof(ctx, "link created: '%s'/'%s' -> '%s'/'%s'", node, port, remoteNode, remotePort)
					} else {
						logger.Infof(ctx, "link destroyed: '%s'/'%s' -> '%s'/'%s'", node, port, remoteNode, remotePort)
					}
				}
			}
		}
	}

	return nil
}

func (p *SimplePlumber) makeLinkState(
	ctx context.Context,
	nodeID0, portID0, nodeID1, portID1 int,
	shouldBeLinked bool,
) (_ret bool, _err error) {
	logger.Debugf(ctx, "makeLinkState(nodeID0=%d, portID0=%d, nodeID1=%d, portID1=%d, shouldBeLinked=%v)",
		nodeID0, portID0, nodeID1, portID1, shouldBeLinked)
	defer func() {
		logger.Tracef(ctx, "/makeLinkState(nodeID0=%d, portID0=%d, nodeID1=%d, portID1=%d, shouldBeLinked=%v): %v %v",
			nodeID0, portID0, nodeID1, portID1, shouldBeLinked, _ret, _err)
	}()

	if nodeID0 == nodeID1 && portID0 == portID1 {
		return false, fmt.Errorf("cannot link a port to itself: nodeID0=%d, portID0=%d, nodeID1=%d, portID1=%d",
			nodeID0, portID0, nodeID1, portID1)
	}

	var linkKey LinkKey
	if _, ok := p.ActiveSources[nodeID0]; ok {
		linkKey = LinkKey{
			From: PortKey{
				NodeID: nodeID0,
				PortID: portID0,
			},
			To: PortKey{
				NodeID: nodeID1,
				PortID: portID1,
			},
		}
	} else if _, ok := p.ActiveSinks[nodeID0]; ok {
		linkKey = LinkKey{
			From: PortKey{
				NodeID: nodeID1,
				PortID: portID1,
			},
			To: PortKey{
				NodeID: nodeID0,
				PortID: portID0,
			},
		}
	} else {
		return false, fmt.Errorf("no source or sink found for node %d", nodeID0)
	}

	if shouldBeLinked {
		if _, ok := p.ActiveLinks[linkKey]; ok {
			logger.Debugf(ctx, "makeLinkState: link already exists, nothing to do")
			return false, nil
		}

		logger.Debugf(ctx, "makeLinkState: creating new link")
		changed, err := p.createLink(ctx, linkKey)
		if err != nil {
			return false, fmt.Errorf("error creating link: %w", err)
		}
		return changed, nil
	}
	if _, ok := p.ActiveLinks[linkKey]; !ok {
		logger.Debugf(ctx, "makeLinkState: link does not exist, nothing to do")
		return false, nil
	}

	logger.Debugf(ctx, "makeLinkState: destroying existing link")
	changed, err := p.destroyLink(ctx, linkKey)
	if err != nil {
		return false, fmt.Errorf("unable to destroy the link: %w", err)
	}

	return changed, nil
}

func (p *SimplePlumber) forgetSink(ctx context.Context, id int) error {
	logger.Debugf(ctx, "forgetSink(%d)", id)
	defer func() { logger.Tracef(ctx, "/forgetSink(%d)", id) }()

	sink, ok := p.ActiveSinks[id]
	if !ok {
		logger.Warnf(ctx, "forgetSink(%d): sink not found", id)
		return nil
	}

	for _, link := range sink.OutboundLinks {
		p.forgetLink(ctx, link.Info)
	}
	for _, port := range sink.Ports {
		p.forgetPort(ctx, port.ID)
	}
	delete(p.ActiveSinks, id)
	return nil
}

func (p *SimplePlumber) forgetSource(ctx context.Context, id int) error {
	logger.Debugf(ctx, "forgetSource(%d)", id)
	defer func() { logger.Tracef(ctx, "/forgetSource(%d)", id) }()

	source, ok := p.ActiveSources[id]
	if !ok {
		logger.Warnf(ctx, "forgetSource(%d): source not found", id)
		return nil
	}

	for _, link := range source.InboundLinks {
		p.forgetLink(ctx, link.Info)
	}
	for _, port := range source.Ports {
		p.forgetPort(ctx, port.ID)
	}
	delete(p.ActiveSources, id)
	return nil
}
