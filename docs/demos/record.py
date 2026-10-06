#!/usr/bin/env python3
"""Record real candidate commands against fresh, offline, synthetic Git repos."""
import argparse
import fcntl
import json
import os
from pathlib import Path
import pty
import select
import shutil
import struct
import subprocess
import tempfile
import termios
import time

ROOT = Path(__file__).resolve().parents[2]
OUT = Path(__file__).resolve().parent


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--wtc', required=True, type=Path)
    parser.add_argument('--agg', type=Path)
    args = parser.parse_args()
    sandbox = Path(tempfile.mkdtemp(prefix='wtc-public-demo-')).resolve()
    bindir = sandbox / 'bin'
    bindir.mkdir()
    for name, target in [('wtc', args.wtc)]:
        (bindir / name).symlink_to(target.resolve())
    # No inherited project environment, credential store selection, prompt, or
    # user Git config. Tool binaries are available without global mise/herdr.
    env = {'PATH': str(bindir) + ':/usr/bin:/bin:/usr/sbin:/sbin',
           'TERM': 'xterm-256color', 'LANG': 'en_US.UTF-8',
           'HOME': os.environ['HOME'],  # Preserve the account home; never redirect it.
           'XDG_CONFIG_HOME': str(sandbox / 'xdg-config'),
           'XDG_DATA_HOME': str(sandbox / 'xdg-data'),
           'GIT_PAGER': 'cat',
           'GIT_CONFIG_NOSYSTEM': '1', 'GIT_CONFIG_GLOBAL': '/dev/null',
           'GIT_AUTHOR_NAME': 'Example', 'GIT_AUTHOR_EMAIL': 'example@example.invalid',
           'GIT_COMMITTER_NAME': 'Example', 'GIT_COMMITTER_EMAIL': 'example@example.invalid',
           'WTC_CONFIG_ROOT': str(sandbox / 'workspace' / '.config')}
    workspace = sandbox / 'workspace'
    sources = sandbox / 'sources'
    sources.mkdir()
    (workspace / '.bare').mkdir(parents=True)

    def run(command, cwd, check=True):
        return subprocess.run(command, cwd=cwd, env=env, check=check,
                              stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True)

    def git(cwd, *args):
        return run(['git', *args], cwd)

    def owner(name):
        source = sources / name
        git(source, 'init', '-b', 'main')
        git(source, 'add', '.')
        git(source, 'commit', '-m', 'Synthetic demo fixture')
        bare = workspace / '.bare' / (name + '.git')
        git(sandbox, 'clone', '--bare', str(source), str(bare))
        git(sandbox, '--git-dir=' + str(bare), 'config', 'remote.origin.fetch',
            '+refs/heads/*:refs/remotes/origin/*')
        git(sandbox, '--git-dir=' + str(bare), 'fetch', 'origin')

    def safe_output(data):
        # Replace only this recording's sandbox path; retain actual command output.
        return data.replace(str(sandbox), '~/wtc-demo')

    class Recording:
        def __init__(self, name, title):
            self.name = name
            self.start = time.monotonic()
            self.events = []
            self.transcript = []
            self.emit('\x1b[2J\x1b[H\x1b[1;36m' + title + '\x1b[0m\r\n\r\n')
            self.transcript.append(title + '\n')

        def emit(self, data):
            self.events.append([round(time.monotonic() - self.start, 3), 'o', data])

        def command(self, command, cwd, expected=0, pause=1.2):
            label = 'workspace' if cwd == workspace else cwd.name
            prompt = f'{label} $ {command}'
            self.emit('\x1b[1;32m' + prompt + '\x1b[0m\r\n')
            self.transcript.append(prompt + '\n')
            master, slave = pty.openpty()
            fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack('HHHH', 32, 110, 0, 0))
            process = subprocess.Popen(['/bin/sh', '-c', command], cwd=cwd,
                                       env=env, stdin=subprocess.DEVNULL,
                                       stdout=slave, stderr=slave, start_new_session=True)
            os.close(slave)
            output = bytearray()
            deadline = time.monotonic() + 45
            while True:
                ready, _, _ = select.select([master], [], [], 0.1)
                if ready:
                    try:
                        data = os.read(master, 65536)
                    except OSError:
                        break
                    if not data:
                        break
                    output.extend(data)
                if time.monotonic() > deadline:
                    process.kill()
                    raise RuntimeError('demo command timed out: ' + command)
            os.close(master)
            code = process.wait(timeout=5)
            text = safe_output(output.decode('utf-8', errors='replace'))
            self.emit(text)
            self.transcript.append(text.replace('\r\n', '\n'))
            if code != expected:
                raise RuntimeError(f'{command}: exit {code}, expected {expected}\n{text}')
            time.sleep(pause)

        def save(self):
            header = {'version': 2, 'width': 110, 'height': 32,
                      'title': self.name, 'env': {'TERM': 'xterm-256color'}}
            cast = OUT / (self.name + '.cast')
            cast.write_text('\n'.join(json.dumps(x) for x in [header, *self.events]) + '\n')
            transcript = ''.join(self.transcript).replace('\r', '')
            (OUT / (self.name + '.txt')).write_text('\n'.join(line.rstrip() for line in transcript.splitlines()) + '\n')
            if args.agg:
                subprocess.run([str(args.agg.resolve()), '--theme', 'github-dark',
                                '--font-size', '17', '--fps-cap', '12',
                                '--last-frame-duration', '3', '--idle-time-limit', '2',
                                str(cast), str(OUT / (self.name + '.gif'))], check=True)

    try:
        # The scaffold is exercised here, outside a collection. The published pin
        # is fixture metadata only: every command uses the supplied candidate.
        harness = sources / 'agent-harness'
        rec = Recording('harness', 'WTC candidate | six-file harness scaffold | no Git, network, or hooks')
        rec.command('wtc harness init agent-harness --remote https://example.invalid/h.git --cli-version 0.1.38', sources, pause=2)
        rec.command('ls -A agent-harness', sources, pause=2)
        rec.command('cat agent-harness/wtc.toml', sources, pause=2)
        rec.command('test ! -d agent-harness/.git && echo "Git setup stays yours"', sources)
        rec.save()
        registry = 'schema_version: 1\nrepositories:\n'
        for name, offset in [('agent-harness', 0), ('api', 1), ('web', 2)]:
            registry += f'  - name: {name}\n    remote: {sources / name}\n    default_ref: origin/main\n'
            if offset:
                registry += f'    port_offset: {offset}\n'
        (harness / '.harness-repos.yml').write_text(registry)
        (sources / 'api').mkdir()
        (sources / 'api' / 'README.md').write_text('# Synthetic API\n')
        (sources / 'web').mkdir()
        (sources / 'web' / 'README.md').write_text('# Synthetic frontend\n')
        for name in ['agent-harness', 'api', 'web']:
            owner(name)
        main_dir = workspace / 'main'
        git(sandbox, '--git-dir=' + str(workspace / '.bare/agent-harness.git'),
            'worktree', 'add', '--detach', str(main_dir / 'harness'), 'origin/main')
        run(['wtc', 'env', 'setup', '--skip-hooks'], main_dir)
        run(['wtc', 'skills', 'render', '--seed-scope'], main_dir)

        rec = Recording('collections', 'WTC candidate | one task, two repositories | offline synthetic fixture')
        rec.command('wtc new add-search api web --no-open', main_dir)
        task = workspace / 'add-search'
        rec.command('ls', task)
        rec.command('wtc status --local --ansi --repos --silent', task, pause=3)
        rec.command("printf '\\nSearch work in progress\\n' >> web/README.md", task)
        rec.command('wtc status --local --ansi --repos --silent', task, pause=3)
        rec.command('git -C web diff --stat', task)
        # Reset only the disposable recording fixture's edit.
        git(task / 'web', 'restore', 'README.md')
        rec.save()

        print('Recorded harness bootstrap and collection commands.')
    finally:
        # Every owner and worktree here belongs to this invocation.
        shutil.rmtree(sandbox)


if __name__ == '__main__':
    main()
