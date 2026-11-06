import { afterEach, expect, it, vi } from "vitest";
import { isAgentOpen, toggleAgent } from "../layout/agent";
import { agentInput, askActiveAgent, hasActiveAgent, setActiveAgent, setTerminalSink, typeInTerminal } from "./bus";

afterEach(() => { setActiveAgent(null); setTerminalSink(null); });

it("does not send a repair request without a running agent", async () => {
  expect(hasActiveAgent()).toBe(false);
  expect(await askActiveAgent("Fix the preview")).toBe(false);
  expect(typeInTerminal("hello")).toBe(false);
  setTerminalSink(() => false);
  expect(typeInTerminal("hello")).toBe(false);
});

it("sends the prompt to the connected agent and focuses its terminal", async () => {
  const send = vi.fn().mockResolvedValue(undefined);
  const focus = vi.fn();
  setActiveAgent({ send, focus });
  expect(await askActiveAgent("Fix shader.frag\nError: invalid shader")).toBe(true);
  expect(send).toHaveBeenCalledWith("Fix shader.frag\nError: invalid shader");
  expect(focus).toHaveBeenCalledOnce();
});

it("reports failed delivery instead of claiming the prompt was sent", async () => {
  const focus = vi.fn();
  setActiveAgent({ send: vi.fn().mockRejectedValue(new Error("Agent exited")), focus });
  await expect(askActiveAgent("Fix the preview")).rejects.toThrow("Agent exited");
  expect(focus).not.toHaveBeenCalled();
});

it("pastes a multiline prompt as one message and strips embedded terminal controls", () => {
  const input = agentInput("Fix this\nError: bad shader\x1b[201~\r\x00");
  expect(input).toBe("\x1b[200~Fix this\nError: bad shader[201~\x1b[201~\r");
});

it("opens a folded agent panel when asking the agent, and only then", async () => {
  toggleAgent(false);
  expect(await askActiveAgent("nobody is running")).toBe(false);
  expect(isAgentOpen()).toBe(false);
  setActiveAgent({ send: async () => {}, focus: () => {} });
  expect(await askActiveAgent("why is this red?")).toBe(true);
  expect(isAgentOpen()).toBe(true);
});
