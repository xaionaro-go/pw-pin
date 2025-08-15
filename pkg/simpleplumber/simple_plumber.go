package simpleplumber

import (
	"context"
	"encoding/json"
	"fmt"

	pwmonitor "github.com/ConnorsApps/pipewire-monitor-go"
	"github.com/facebookincubator/go-belt/tool/logger"
	"github.com/go-ng/xatomic"
	"github.com/xaionaro-go/observability"
	"github.com/xaionaro-go/xsync"
)

type Port struct {
	ID   int
	Info *pwmonitor.EventInfo
}

type Node struct {
	Info  *pwmonitor.EventInfo
	Ports map[int]*Port
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
	Input  PortKey
	Output PortKey
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

func (p *SimplePlumber) SetConfig(config *Config) {
	xatomic.StorePointer(&p.Config, config)
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
		return p.deleteLink(ctx, e.Info)
	default:
		return p.addOrUpdateLink(ctx, e)
	}
}

func (p *SimplePlumber) deleteLink(ctx context.Context, eInfo *pwmonitor.EventInfo) error {
	logger.Debugf(ctx, "deleteLink(%s)", jsoninze(eInfo))
	defer func() { logger.Tracef(ctx, "/deleteLink(%s)", jsoninze(eInfo)) }()
	linkKey := LinkKey{
		Input: PortKey{
			NodeID: *eInfo.Props.LinkInputNode,
			PortID: *eInfo.Props.LinkInputPort,
		},
		Output: PortKey{
			NodeID: *eInfo.Props.LinkOutputNode,
			PortID: *eInfo.Props.LinkOutputPort,
		},
	}
	if _, ok := p.ActiveLinks[linkKey]; !ok {
		logger.Warnf(ctx, "deleteLink(%s): link not found", jsoninze(eInfo))
		return nil
	}
	delete(p.ActiveSources[linkKey.Input.NodeID].InboundLinks, linkKey.Output)
	delete(p.ActiveSinks[linkKey.Output.NodeID].OutboundLinks, linkKey.Input)
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
		Input: PortKey{
			NodeID: *e.Info.Props.LinkInputNode,
			PortID: *e.Info.Props.LinkInputPort,
		},
		Output: PortKey{
			NodeID: *e.Info.Props.LinkOutputNode,
			PortID: *e.Info.Props.LinkOutputPort,
		},
	}

	if _, ok := p.ActiveLinks[linkKey]; ok {
		logger.Debugf(ctx, "processEventLink(%s): link already exists", jsoninze(e))
		return nil
	}

	nodeInput := p.ActiveSources[linkKey.Input.NodeID]
	if nodeInput == nil {
		logger.Warnf(ctx, "processEventLink(%s): source node %d not found", jsoninze(e), linkKey.Input.NodeID)
		return nil
	}
	nodeOutput := p.ActiveSinks[linkKey.Output.NodeID]
	if nodeOutput == nil {
		logger.Warnf(ctx, "processEventLink(%s): sink node %d not found", jsoninze(e), linkKey.Output.NodeID)
		return nil
	}

	logger.Debugf(ctx, "processEventLink(%s): new link", jsoninze(e))
	link := &Link{Info: e.Info}
	p.ActiveLinks[linkKey] = link
	nodeInput.InboundLinks[linkKey.Output] = link
	nodeOutput.OutboundLinks[linkKey.Input] = link
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
		return p.deleteSink(ctx, e.ID)
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
		return p.deleteSource(ctx, e.ID)
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
		return p.deleteSink(ctx, e.ID)
	default:
		return p.addOrUpdateSink(ctx, e)
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
		return p.deleteSource(ctx, e.ID)
	default:
		return p.addOrUpdateSource(ctx, e)
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
		return p.deletePort(ctx, e.ID)
	default:
		return p.addOrUpdatePort(ctx, e)
	}
}

func (p *SimplePlumber) deletePort(ctx context.Context, portID int) (_err error) {
	logger.Debugf(ctx, "deletePort(%d)", portID)
	defer func() { logger.Tracef(ctx, "/deletePort(%d)", portID, _err) }()

	port := p.ActivePorts[portID]
	if port == nil {
		logger.Warnf(ctx, "deletePort(%d): port not found", portID)
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
		logger.Tracef(ctx, "deletePort(%d): no node found for the port event", portID)
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
	return nil
}

func (p *SimplePlumber) deleteSink(ctx context.Context, id int) error {
	logger.Debugf(ctx, "deleteSink(%d)", id)
	defer func() { logger.Tracef(ctx, "/deleteSink(%d)", id) }()

	sink, ok := p.ActiveSinks[id]
	if !ok {
		logger.Warnf(ctx, "deleteSink(%d): sink not found", id)
		return nil
	}

	for _, link := range sink.OutboundLinks {
		p.deleteLink(ctx, link.Info)
	}
	for _, port := range sink.Ports {
		p.deletePort(ctx, port.ID)
	}
	delete(p.ActiveSinks, id)
	return nil
}

func (p *SimplePlumber) deleteSource(ctx context.Context, id int) error {
	logger.Debugf(ctx, "deleteSource(%d)", id)
	defer func() { logger.Tracef(ctx, "/deleteSource(%d)", id) }()

	source, ok := p.ActiveSources[id]
	if !ok {
		logger.Warnf(ctx, "deleteSource(%d): source not found", id)
		return nil
	}

	for _, link := range source.InboundLinks {
		p.deleteLink(ctx, link.Info)
	}
	for _, port := range source.Ports {
		p.deletePort(ctx, port.ID)
	}
	delete(p.ActiveSources, id)
	return nil
}
