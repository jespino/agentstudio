import type { ReactNode } from 'react';
import { Group, Panel, Separator } from 'react-resizable-panels';
import { TerminalPanel } from '../terminal/TerminalPanel';

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
}

/** The studio's split: its own work on the left, the agent on the right. */
export function StudioLayout({ children, agent, agentSize = '34', mainMinSize = 420, agentMinSize = 320 }: StudioLayoutProps) {
  return (
    <Group orientation="horizontal" className="main">
      <Panel id="main" minSize={mainMinSize}>
        {children}
      </Panel>
      <Separator className="sep-v" />
      <Panel id="agent" defaultSize={agentSize} minSize={agentMinSize}>
        {agent ?? <TerminalPanel />}
      </Panel>
    </Group>
  );
}
