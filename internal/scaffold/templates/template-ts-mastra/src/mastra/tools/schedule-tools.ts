import { createTool } from '@mastra/core/tools';
import { z } from 'zod';
import { SCHEDULE_DELIVERY_KEY } from '../deliver-scheduled-output';

/** This agent's id, as registered in src/mastra/index.ts. */
const AGENT_ID = '{{.Name}}';

export const startScheduleTool = createTool({
  id: 'start_schedule',
  description: 'Start a recurring schedule for this conversation.',
  inputSchema: z.object({
    schedule: z
      .string()
      .describe(
        'Cron expression for when to run. Accepts 5, 6 (with seconds), or 7 fields.',
      ),
    prompt: z.string().describe('Prompt to run on the schedule.'),
  }),
  execute: async ({ schedule, prompt }, { mastra, agent }) => {
    if (!agent?.threadId || !agent.resourceId) {
      throw new Error('A threadId and resourceId are required to create a schedule.');
    }

    return mastra!.schedules.create({
      agentId: AGENT_ID,
      cron: schedule,
      prompt,
      threadId: agent.threadId,
      resourceId: agent.resourceId,
      // A threaded schedule fires as a notification signal, and the default
      // notification policy is priority-aware: a low-priority notification is
      // batched or held until the thread is next used rather than waking it. That
      // records a fire with no run and no output, which reads as a broken schedule.
      // 'wake' runs the agent immediately, and the request context tells the
      // deliver-scheduled-output processor where to send the result.
      ifIdle: {
        behavior: 'wake',
        streamOptions: {
          requestContext: { [SCHEDULE_DELIVERY_KEY]: agent.threadId },
        },
      },
      // If the user is already streaming, drop the prompt into the running loop so
      // they see it inline instead of receiving a duplicate push.
      ifActive: { behavior: 'deliver' },
    });
  },
});

export const listSchedulesTool = createTool({
  id: 'list_schedules',
  description:
    "List this user's schedules, with their ids and whether each is active. Use this to answer what is scheduled and to find the id needed to stop one.",
  inputSchema: z.object({}),
  execute: async (_input, { mastra, agent }) => {
    if (!agent?.resourceId) {
      throw new Error('A resourceId is required to list schedules.');
    }

    // Scoped to the caller's resourceId: a schedule belonging to another user must
    // not be listed, and an id the caller never saw cannot be stopped.
    const schedules = await mastra!.schedules.list({
      agentId: AGENT_ID,
      resourceId: agent.resourceId,
    });

    return schedules.map((s) => ({
      id: s.id,
      cron: s.cron,
      status: s.status,
      prompt: 'prompt' in s ? s.prompt : undefined,
      nextFireAt: new Date(s.nextFireAt).toISOString(),
      lastFireAt: s.lastFireAt ? new Date(s.lastFireAt).toISOString() : undefined,
    }));
  },
});

export const stopScheduleTool = createTool({
  id: 'stop_schedule',
  description: 'Stop a schedule by pausing it. Use list_schedules to find the id.',
  inputSchema: z.object({
    scheduleId: z.string().describe('Schedule id from start_schedule or list_schedules.'),
  }),
  execute: async ({ scheduleId }, { mastra, agent }) => {
    if (!agent?.resourceId) {
      throw new Error('A resourceId is required to stop a schedule.');
    }

    // Schedule ids are guessable and the pause API is not scoped to a caller, so
    // check ownership here — otherwise one user could pause another's schedule.
    // Report a missing schedule and a foreign one identically, so this cannot be
    // used to probe which ids exist.
    // get() returns agent and workflow schedules; only an agent schedule carries a
    // resourceId, so the agentId check both narrows the union and rejects a
    // workflow-schedule id.
    const schedule = await mastra!.schedules.get(scheduleId);
    const owned =
      schedule?.agentId !== undefined && schedule.resourceId === agent.resourceId;
    if (!owned) {
      throw new Error(`No schedule ${scheduleId} belongs to this conversation.`);
    }

    return mastra!.schedules.pause(scheduleId);
  },
});
