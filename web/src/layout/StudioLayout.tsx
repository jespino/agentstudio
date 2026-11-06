import { useEffect, useRef, useState, useSyncExternalStore, type ReactNode } from 'react';
import { Group, Panel, Separator, usePanelRef } from 'react-resizable-panels';
import { TerminalPanel } from '../terminal/TerminalPanel';
import { getActiveAgent, subscribeToAgent } from '../terminal/bus';
import { AgentFoldIcon, AgentFolds, toggleAgent, useAgentOpen } from './agent';

export interface StudioLayoutProps {
  /** The studio's own side: the preview, the views, the editor… */
  children: ReactNode;
  /** The agent side; a TerminalPanel by default. */
  agent?: ReactNode;
  /** The agent side's initial share of the width, in percent. */
  agentSize?: string;
  /** Minimum widths in pixels. */
  mainMinSize?: number;
  agentMinSize?: number;
  /** The agent side can fold to a rail on the right (AgentButton, Ctrl+3). */
  collapsible?: boolean;
}

/** The folded agent panel's width, in pixels. */
const RAIL = 30;

/** The studio's split: its own work on the left, the agent on the right. */
export function StudioLayout({ children, agent, agentSize = '34', mainMinSize = 420, agentMinSize = 320, collapsible = false }: StudioLayoutProps) {
  const body = agent ?? <TerminalPanel />;
  if (!collapsible) {
    return (
      <Group orientation="horizontal" className="main">
        <Panel id="main" minSize={mainMinSize}>
          {children}
        </Panel>
        <Separator className="sep-v" />
        <Panel id="agent" defaultSize={agentSize} minSize={agentMinSize}>
          {body}
        </Panel>
      </Group>
    );
  }
  return (
    <Collapsible agentSize={agentSize} mainMinSize={mainMinSize} agentMinSize={agentMinSize} agent={body}>
      {children}
    </Collapsible>
  );
}

function Collapsible({ children, agent, agentSize, mainMinSize, agentMinSize }: Required<Omit<StudioLayoutProps, 'collapsible'>>) {
  const panel = usePanelRef();
  const open = useAgentOpen();
  const running = useSyncExternalStore(subscribeToAgent, getActiveAgent) !== null;
  // the width to give the agent back; also what it keeps while folded
  const width = useRef(Math.max(agentMinSize, Math.round((window.innerWidth * parseFloat(agentSize)) / 100)));
  // The agent keeps its width from before folding until the panel has grown
  // back: a terminal that follows the panel down to the rail would tell the
  // program in it that it has a few columns, and it would redraw for them.
  const [frozen, setFrozen] = useState(!open);
  const wasOpen = useRef(open);

  useEffect(() => {
    const p = panel.current;
    if (!p) return;
    if (!open && !p.isCollapsed()) p.collapse();
    if (open && p.isCollapsed()) {
      p.expand();
      p.resize(width.current);
    }
    // back to the terminal, also when it runs a shell rather than an agent
    if (open && !wasOpen.current) document.querySelector<HTMLElement>('.agent-slot .xterm textarea')?.focus();
    wasOpen.current = open;
  }, [open, panel]);

  return (
    <Group orientation="horizontal" className="main">
      <Panel id="main" minSize={mainMinSize}>
        {children}
      </Panel>
      <Separator className="sep-v" />
      <Panel
        id="agent"
        className="agent-panel"
        panelRef={panel}
        collapsible
        collapsedSize={RAIL}
        // folded at the start, or the first resize would read as the user opening it
        defaultSize={open ? agentSize : RAIL}
        minSize={agentMinSize}
        onResize={(size) => {
          const collapsed = panel.current?.isCollapsed();
          if (collapsed === undefined) return;
          if (collapsed === open) toggleAgent(!collapsed); // dragged past its minimum
          if (collapsed) setFrozen(true);
          else if (size.inPixels >= agentMinSize) {
            width.current = size.inPixels;
            setFrozen(false);
          }
        }}
      >
        <div className="agent-slot" style={!open || frozen ? { width: width.current } : undefined} inert={!open}>
          <AgentFolds.Provider value>{agent}</AgentFolds.Provider>
        </div>
        {!open && (
          <button className="agent-rail" onClick={() => toggleAgent(true)} title="Show the agent (Ctrl+3)">
            <AgentFoldIcon fold={false} />
            <span className="agent-rail-label">Agent</span>
            {running && <i title="An agent is running" />}
          </button>
        )}
      </Panel>
    </Group>
  );
}
