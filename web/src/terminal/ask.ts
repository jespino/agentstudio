// Asking the agent in the terminal panel, from anywhere in the UI ("Ask the
// agent to fix it" on a compile error, a preview failure, …).
import { useEffect, useState, useSyncExternalStore } from 'react';
import { errorText } from '../backend';
import { toast } from '../toast';
import { askActiveAgent, focusAgentPanel, getActiveAgent, subscribeToAgent } from './bus';

/**
 * Types prompt into the running agent. With none running it shows the agent
 * panel and says to start one (a toast); it returns whether the prompt went.
 */
export async function askAgent(prompt: string, noAgent = 'Start an agent in the Agent panel first.'): Promise<boolean> {
  if (await askActiveAgent(prompt)) return true;
  focusAgentPanel();
  toast(noAgent);
  return false;
}

export interface AskAgent {
  /** an agent is running (a shell doesn't count) */
  available: boolean;
  sending: boolean;
  /** the prompt went to the running agent (until `reset` changes or another agent starts) */
  sent: boolean;
  /** why it didn't go */
  error: string;
  /** sends the prompt to the running agent */
  ask: (prompt: string) => Promise<void>;
  /** shows the agent panel, to start one */
  open: () => void;
}

/**
 * The state of a button that sends something to the agent. `reset` (the
 * error's text, say) clears "sent" when it changes.
 */
export function useAskAgent(reset?: unknown): AskAgent {
  const agent = useSyncExternalStore(subscribeToAgent, getActiveAgent);
  const [sending, setSending] = useState(false);
  const [sentTo, setSentTo] = useState<typeof agent>(null);
  const [error, setError] = useState('');
  useEffect(() => {
    setSentTo(null);
    setError('');
  }, [reset]);
  const ask = async (prompt: string) => {
    setSending(true);
    setError('');
    const target = agent;
    try {
      if (await askActiveAgent(prompt)) setSentTo(target);
      else focusAgentPanel();
    } catch (e) {
      setError(`Could not send to your agent: ${errorText(e)}`);
    } finally {
      setSending(false);
    }
  };
  return { available: agent !== null, sending, sent: agent !== null && sentTo === agent, error, ask, open: focusAgentPanel };
}
