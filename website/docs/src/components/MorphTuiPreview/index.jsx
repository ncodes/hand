import { useEffect, useRef, useState } from 'react';

const SCRIPT = [
  {type: 'user', text: 'prepare Slack gateway rollout'},
  {type: 'morph', lines: ['I will inspect the profile, gateway docs, and current daemon readiness first.']},
  {type: 'tool', title: 'Plan updated', lines: ['└ Added 3 tasks']},
  {type: 'tool', title: 'Read', lines: ['├ read_file .morph/profiles/default/config.yaml', '└ read_file docs/guides/gateway/slack.md']},
  {type: 'tool', title: 'Run 1 shell command', lines: ['└ $ morph doctor --json']},
  {type: 'tool', title: 'Doctor result', lines: ['├ profile: default ready', '├ models: main provider ready', '└ daemon: restart recommended']},
  {type: 'morph', lines: ['Created a rollout checklist for the Slack gateway:', '• morph doctor is passing for models and profile config', '• gateway.slack.botToken is present', '• Restart the daemon before opening the shared channel']},
  {type: 'user', text: 'tighten this for shared channels'},
  {type: 'morph', lines: ['I will make the checklist safer for a channel where more than one person can invoke Morph.']},
  {type: 'tool', title: 'Plan updated', lines: ['└ Task 2 refined for shared-channel controls']},
  {type: 'tool', title: 'Read', lines: ['├ read_file docs/operations/security.md', '└ read_file docs/operations/gateway-management.md']},
  {type: 'tool', title: 'Config check', lines: ['├ gateway.authToken is configured', '├ slack signing secret is configured', '└ non-loopback HTTP mode is disabled']},
  {type: 'morph', lines: ['Updated the checklist for a shared Slack channel:', '• Limit who can invoke Morph in the channel', '• Keep gateway auth enabled before exposing HTTP mode', '• Announce the restart window before reconnecting Slack']},
  {type: 'user', text: 'add an HTTP fallback plan'},
  {type: 'morph', lines: ['I will add an HTTP fallback without making it the default path.']},
  {type: 'tool', title: 'Read', lines: ['└ read_file docs/guides/gateway/http.md']},
  {type: 'tool', title: 'Run 1 shell command', lines: ['└ $ morph gateway status']},
  {type: 'tool', title: 'Gateway result', lines: ['├ slack: ready after daemon restart', '├ http: disabled', '└ telegram: not configured']},
  {type: 'morph', lines: ['HTTP fallback added:', '• Keep HTTP bound to loopback during rehearsal', '• Require gateway.authToken before widening the bind address', '• Prefer Slack socket mode for the shared channel launch']},
  {type: 'morph', lines: ['Final rollout path: verify doctor, restart daemon, open Slack, confirm one channel, then keep HTTP as a guarded fallback.']},
];

const TYPE_DELAY = 18;
const LINE_DELAY = 120;
const EVENT_DELAY = 280;
const REPLAY_DELAY = 4200;
const START_DELAY = 1800;

function wait(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

function renderEvent(event) {
  if (event.type === 'user') {
    return (
      <div className="morphTuiPreview__event morphTuiPreview__eventUser" key={event.id}>
        <span className="morphDocsHome__prompt">›</span>
        <span>{event.text}</span>
      </div>
    );
  }

  if (event.type === 'tool') {
    return (
      <div className="morphTuiPreview__event morphTuiPreview__eventTool" key={event.id}>
        <span className="morphDocsHome__logDot"></span>
        <div className="morphTuiPreview__toolBody">
          {event.title ? <strong>{event.title}</strong> : null}
          {event.lines.map((line, index) => (
            <span key={`${event.id}-${index}`}>{line}</span>
          ))}
        </div>
      </div>
    );
  }

  return (
    <div className="morphTuiPreview__event morphTuiPreview__eventMorph" key={event.id}>
      {event.lines.map((line, index) => (
        <span key={`${event.id}-${index}`}>{line}</span>
      ))}
    </div>
  );
}

export default function MorphTuiPreview() {
  const [events, setEvents] = useState([]);
  const [composerText, setComposerText] = useState('');
  const viewportRef = useRef(null);

  useEffect(() => {
    const viewport = viewportRef.current;
    if (!viewport) return;
    viewport.scrollTo({top: viewport.scrollHeight, behavior: 'smooth'});
  }, [events, composerText]);

  useEffect(() => {
    let cancelled = false;
    let runId = 0;

    const updateEvent = (id, updater) => {
      setEvents((current) => current.map((event) => (event.id === id ? updater(event) : event)));
    };

    const appendTitleChar = (id, char) => {
      updateEvent(id, (event) => ({...event, title: `${event.title}${char}`}));
    };

    const appendLine = (id) => {
      updateEvent(id, (event) => ({...event, lines: [...event.lines, '']}));
    };

    const appendLineChar = (id, lineIndex, char) => {
      updateEvent(id, (event) => {
        const nextLines = [...event.lines];
        nextLines[lineIndex] = `${nextLines[lineIndex] ?? ''}${char}`;
        return {...event, lines: nextLines};
      });
    };

    const typeComposer = async (text) => {
      setComposerText('');
      for (let index = 0; index < text.length; index += 1) {
        if (cancelled) return;
        setComposerText((current) => `${current}${text[index]}`);
        await wait(TYPE_DELAY);
      }
      await wait(180);
    };

    const typeEvent = async (entry, id) => {
      if (entry.type === 'tool') {
        setEvents((current) => [...current, {id, type: 'tool', title: '', lines: []}]);

        for (let index = 0; index < entry.title.length; index += 1) {
          if (cancelled) return;
          appendTitleChar(id, entry.title[index]);
          await wait(TYPE_DELAY);
        }

        await wait(LINE_DELAY);
      } else {
        setEvents((current) => [...current, {id, type: 'morph', lines: []}]);
      }

      for (let lineIndex = 0; lineIndex < entry.lines.length; lineIndex += 1) {
        const line = entry.lines[lineIndex];
        appendLine(id);

        for (let index = 0; index < line.length; index += 1) {
          if (cancelled) return;
          appendLineChar(id, lineIndex, line[index]);
          await wait(TYPE_DELAY);
        }

        await wait(LINE_DELAY);
      }
    };

    const run = async () => {
      while (!cancelled) {
        runId += 1;
        setEvents([]);
        setComposerText('');
        await wait(START_DELAY);

        for (let index = 0; index < SCRIPT.length; index += 1) {
          if (cancelled) return;
          const entry = SCRIPT[index];
          const id = `${runId}-${index}`;

          if (entry.type === 'user') {
            await typeComposer(entry.text);
            if (cancelled) return;
            setEvents((current) => [...current, {id, type: 'user', text: entry.text}]);
            setComposerText('');
          } else {
            await typeEvent(entry, id);
          }

          await wait(EVENT_DELAY);
        }

        await wait(REPLAY_DELAY);
      }
    };

    run();

    return () => {
      cancelled = true;
    };
  }, []);

  return (
    <div className="morphDocsHome__tui">
      <div className="morphDocsHome__tuiHeader">
        <span>Morph TUI</span>
        <span>profile: support</span>
      </div>
      <div className="morphDocsHome__tuiViewport" ref={viewportRef}>
        <div className="morphTuiPreview__transcript">
          {events.map(renderEvent)}
        </div>
      </div>
      <div className="morphDocsHome__composer">
        <span className="morphDocsHome__prompt">›</span>
        <span className="morphTuiPreview__composerText">
          {composerText ? composerText : <span className="morphTuiPreview__placeholder">Ask Morph...</span>}
          <span className="morphDocsHome__caret" aria-hidden="true"></span>
        </span>
      </div>
      <div className="morphDocsHome__tuiStatus" aria-hidden="true">
        <span>gpt-5.5</span>
        <span>enter to send</span>
        <span>ctrl+c to quit</span>
        <span>12,840 used · 18%</span>
      </div>
    </div>
  );
}
