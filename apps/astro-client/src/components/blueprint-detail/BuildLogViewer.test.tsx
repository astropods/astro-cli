import { render, screen, fireEvent } from '@testing-library/react';
import { describe, it, expect } from 'vitest';
import { BuildLogViewer } from './BuildLogViewer';
import type { BuildLogComponentData } from './BuildLogViewer';

const LOGS = `=== git-clone ===
Cloning into '/workspace'...
remote: Counting objects: 100% (142/142), done.
=== buildkit ===
[1/4] FROM docker.io/library/python:3.11-slim
[2/4] COPY requirements.txt .
ERROR: failed to solve: open Dockerfile: no such file or directory`;

function makeComp(name: string, status: BuildLogComponentData["status"], logs = LOGS): BuildLogComponentData {
  return { name, status, logs };
}

describe('BuildLogViewer', () => {
  it('shows loading state', () => {
    render(<BuildLogViewer isLoading components={[]} />);
    expect(screen.getByText(/loading logs/i)).toBeInTheDocument();
  });

  it('shows error state', () => {
    render(<BuildLogViewer error="Logs unavailable. The pod may have been cleaned up." components={[]} />);
    expect(screen.getByText(/logs unavailable/i)).toBeInTheDocument();
  });

  it('shows no output state with header', () => {
    render(<BuildLogViewer commitSha="a1b2c3d" buildId="bld_abc123" components={[]} />);
    expect(screen.getByText('Build Logs')).toBeInTheDocument();
    expect(screen.getByText(/no output/i)).toBeInTheDocument();
  });

  it('renders header with commit sha and build id', () => {
    render(<BuildLogViewer commitSha="a1b2c3d" buildId="bld_abc123" components={[makeComp('agent', 'succeeded')]} />);
    expect(screen.getByText('a1b2c3d')).toBeInTheDocument();
    expect(screen.getByText('bld_abc123')).toBeInTheDocument();
  });

  it('shows pending when buildId is absent', () => {
    render(<BuildLogViewer commitSha="a1b2c3d" components={[makeComp('agent', 'building')]} />);
    expect(screen.getByText('pending')).toBeInTheDocument();
  });

  it('shows total duration in header', () => {
    render(<BuildLogViewer commitSha="a1b2c3d" buildId="bld_abc123" totalDuration="3m 23s" components={[makeComp('agent', 'succeeded')]} />);
    expect(screen.getByText('total duration: 3m 23s')).toBeInTheDocument();
  });

  it('shows label row for single component instead of tabs', () => {
    render(<BuildLogViewer components={[makeComp('agent', 'succeeded')]} />);
    expect(screen.getByText('agent')).toBeInTheDocument();
    // only one "agent" element — no tab bar
    expect(screen.getAllByText('agent')).toHaveLength(1);
  });

  it('renders tabs for multiple components', () => {
    render(<BuildLogViewer components={[makeComp('agent', 'succeeded'), makeComp('ingestion-startup', 'building')]} />);
    expect(screen.getByRole('button', { name: /agent/i })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /ingestion-startup/i })).toBeInTheDocument();
  });

  it('switches tab on click', () => {
    render(<BuildLogViewer components={[makeComp('agent', 'succeeded'), makeComp('ingestion-startup', 'succeeded')]} />);
    fireEvent.click(screen.getByRole('button', { name: /ingestion-startup/i }));
    // sections for ingestion-startup should now be visible
    expect(screen.getAllByRole('button').some(b => b.textContent?.includes('git-clone'))).toBe(true);
  });

  it('filters out events and ecr-login sections', () => {
    const logs = `=== events ===\nPulling image\n=== ecr-login ===\nLogin Succeeded\n=== git-clone ===\nCloning...`;
    render(<BuildLogViewer components={[makeComp('agent', 'succeeded', logs)]} />);
    expect(screen.queryByText('events')).not.toBeInTheDocument();
    expect(screen.queryByText('ecr-login')).not.toBeInTheDocument();
    expect(screen.getByText('git-clone')).toBeInTheDocument();
  });

  it('renames buildkit section to build', () => {
    const logs = `=== buildkit ===\n[1/4] FROM python:3.11`;
    render(<BuildLogViewer components={[makeComp('agent', 'succeeded', logs)]} />);
    expect(screen.queryByText('buildkit')).not.toBeInTheDocument();
    expect(screen.getByText('build')).toBeInTheDocument();
  });

  it('expands section and shows log lines on click', () => {
    render(<BuildLogViewer components={[makeComp('agent', 'succeeded')]} />);
    fireEvent.click(screen.getByText('git-clone'));
    expect(screen.getByText(/Cloning into/)).toBeInTheDocument();
  });

  it('shows component duration in tab', () => {
    render(<BuildLogViewer components={[{ ...makeComp('agent', 'succeeded'), duration: '1m 12s' }, makeComp('ingestion-startup', 'building')]} />);
    expect(screen.getByText('1m 12s')).toBeInTheDocument();
  });
});
