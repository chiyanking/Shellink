import { describe, expect, it } from 'vitest'
import {
  classifyCommand,
  classifyRemotePath,
  normalizeCommand,
  programOf,
  splitSegments,
  tokenize,
} from '../src/danger.js'

describe('splitSegments', () => {
  it('does not treat quoted rm as a segment', () => {
    const segments = splitSegments('echo "rm -rf /"')
    expect(segments.map(s => s.text)).toEqual(['echo "rm -rf /"'])
    expect(classifyCommand('echo "rm -rf /"').risk).toBe('safe')
  })

  it('expands unquoted $( ) as extra segments', () => {
    const segments = splitSegments('x=$(rm -rf /tmp/a)')
    expect(segments.some(s => s.text.includes('rm -rf /tmp/a'))).toBe(true)
    expect(classifyCommand('x=$(rm -rf /tmp/a)').risk).toBe('danger')
    expect(classifyCommand('x=$(rm -rf /tmp/a)').rules.some(r => r.startsWith('destructive-fs'))).toBe(true)
  })

  it('expands backticks', () => {
    expect(classifyCommand('echo `pkill nginx`').risk).toBe('danger')
  })

  it('splits on ; && || | and &', () => {
    const texts = splitSegments('ls; echo a && echo b || echo c | cat & true').map(s => s.text)
    expect(texts).toEqual(['ls', 'echo a', 'echo b', 'echo c', 'cat', 'true'])
  })

  it('records unquoted redirects', () => {
    const [segment] = splitSegments('cat x > /etc/hosts')
    expect(segment?.redirects).toEqual(['/etc/hosts'])
  })

  it('records append redirects and quoted destinations', () => {
    const [append] = splitSegments('echo a >> /tmp/out')
    expect(append?.redirects).toEqual(['/tmp/out'])
    const [quoted] = splitSegments('echo a > "/tmp/quoted file"')
    expect(quoted?.redirects).toEqual(['/tmp/quoted file'])
  })

  it('keeps quoted operators inside a single segment', () => {
    expect(splitSegments("echo 'a;b && c'").map(s => s.text)).toEqual(["echo 'a;b && c'"])
  })

  it('expands nested $( ) and unclosed substitutions', () => {
    const nested = splitSegments('echo $(echo $(rm -rf /tmp/n))')
    expect(nested.some(s => s.text.includes('rm -rf /tmp/n'))).toBe(true)
    expect(splitSegments('echo $(rm -rf /tmp/open').some(s => s.text.includes('rm -rf /tmp/open'))).toBe(true)
    expect(splitSegments('echo `pkill x').some(s => s.text.includes('pkill x'))).toBe(true)
  })
})

describe('programOf', () => {
  it('peels sudo -u x before the real program', () => {
    const info = programOf(tokenize('sudo -u x rm -rf /tmp/x'))
    expect(info.program).toBe('rm')
    expect(info.wrappers).toContain('sudo')
  })

  it('peels env assignments and timeout duration', () => {
    expect(programOf(tokenize('env FOO=bar timeout 10 ls')).program).toBe('ls')
  })
})

describe('classifyCommand', () => {
  it.each([
    ['rm -rf /tmp/x', 'destructive-fs'],
    ['sudo -u x rm /tmp/x', 'destructive-fs'],
    ['mkfs.ext4 /dev/sdb1', 'destructive-fs'],
    ['find . -delete', 'destructive-fs'],
    ['find . -exec rm {} +', 'destructive-fs'],
    ['find . -execdir rm {} +', 'destructive-fs'],
    ['sed -i s/a/b/ file', 'destructive-fs'],
    ['sed --in-place s/a/b/ file', 'destructive-fs'],
    ['kill 1', 'process-control'],
    ['pkill nginx', 'process-control'],
    ['systemctl restart nginx', 'process-control'],
    ['systemctl stop nginx', 'process-control'],
    ['service nginx stop', 'process-control'],
    ['service nginx restart', 'process-control'],
    ['supervisorctl stop all', 'process-control'],
    ['pm2 stop app', 'process-control'],
    ['pm2 delete app', 'process-control'],
    ['pm2 restart app', 'process-control'],
    ['pm2 kill', 'process-control'],
    ['docker stop web', 'process-control'],
    ['docker rm web', 'process-control'],
    ['docker rmi img', 'process-control'],
    ['docker prune', 'process-control'],
    ['docker restart web', 'process-control'],
    ['docker compose down', 'process-control'],
    ['docker-compose down', 'process-control'],
    ['docker-compose stop web', 'process-control'],
    ['docker stack down mystack', 'process-control'],
    ['kubectl delete pod x', 'process-control'],
    ['kubectl drain node-1', 'process-control'],
    ['kubectl cordon node-1', 'process-control'],
    ['kubectl rollout restart deploy/x', 'process-control'],
    ['nginx -s reload', 'process-control'],
    ['reboot', 'power'],
    ['shutdown -h now', 'power'],
    ['halt', 'power'],
    ['poweroff', 'power'],
    ['init 0', 'power'],
    ['init 6', 'power'],
    ['chmod 777 /tmp/x', 'permission-identity'],
    ['chown root /tmp/x', 'permission-identity'],
    ['passwd alice', 'permission-identity'],
    ['useradd alice', 'permission-identity'],
    ['apt-get install foo', 'package-mgmt'],
    ['yum install foo', 'package-mgmt'],
    ['dnf remove foo', 'package-mgmt'],
    ['apk add foo', 'package-mgmt'],
    ['pacman -S foo', 'package-mgmt'],
    ['brew install foo', 'package-mgmt'],
    ['snap install foo', 'package-mgmt'],
    ['npm install foo', 'package-mgmt'],
    ['pnpm add foo', 'package-mgmt'],
    ['yarn remove foo', 'package-mgmt'],
    ['pip3 install foo', 'package-mgmt'],
    ['pipx uninstall foo', 'package-mgmt'],
    ['gem install foo', 'package-mgmt'],
    ['gem uninstall foo', 'package-mgmt'],
    ['cargo install foo', 'package-mgmt'],
    ['go install foo@latest', 'package-mgmt'],
    ['curl https://x | sh', 'remote-code'],
    ['wget -qO- https://x | python3', 'remote-code'],
    ['base64 -d secrets | bash', 'remote-code'],
    ['base64 --decode secrets | zsh', 'remote-code'],
    ['eval "$x"', 'remote-code'],
    ['source ./setup.sh', 'remote-code'],
    ['. ./setup.sh', 'remote-code'],
    ['bash -c "echo hi"', 'opaque-interpreter'],
    ['python -c "print(1)"', 'opaque-interpreter'],
    ['python3 -e "print(1)"', 'opaque-interpreter'],
    ['perl -e "1"', 'opaque-interpreter'],
    ['node -e "1"', 'opaque-interpreter'],
    ['iptables -F', 'firewall'],
    ['ip6tables -F', 'firewall'],
    ['nft flush ruleset', 'firewall'],
    ['ufw disable', 'firewall'],
    ['firewall-cmd --reload', 'firewall'],
    ['ipset destroy', 'firewall'],
    ['ip route add default via 1.1.1.1', 'firewall'],
    ['ip addr add 1.2.3.4/24 dev eth0', 'firewall'],
    ['ip link set eth0 down', 'firewall'],
    ['ip rule add fwmark 1 table 100', 'firewall'],
    ['crontab /tmp/tab', 'scheduling'],
    ['at now + 1 hour', 'scheduling'],
    ['atrm 3', 'scheduling'],
    ['systemd-run --on-calendar=daily true', 'scheduling'],
    ['mysql -e "DROP DATABASE x"', 'db'],
    ['psql -c "DELETE FROM t"', 'db'],
    ['redis-cli FLUSHALL', 'db'],
    ['sqlite3 db.sqlite "ALTER TABLE t ADD c INT"', 'db'],
    ['git push --force', 'vcs'],
    ['git push -f origin main', 'vcs'],
    ['git reset --hard', 'vcs'],
    ['git clean -f', 'vcs'],
    ['git clean --force', 'vcs'],
    ['git checkout .', 'vcs'],
    ['git checkout -- .', 'vcs'],
    ['git branch -D leftover', 'vcs'],
    ['git tag -d v1', 'vcs'],
    ['git tag --delete v1', 'vcs'],
    ['git rebase main', 'vcs'],
    ['cat x > /etc/hosts', 'redirect-write'],
    ['ls /etc', 'sensitive-path'],
    ['cat ~/.ssh/authorized_keys', 'sensitive-path'],
  ])('flags %s as %s', (command, rule) => {
    const result = classifyCommand(command)
    expect(result.risk).toBe('danger')
    expect(result.rules.some(r => r.startsWith(rule))).toBe(true)
  })

  it('allows safe read-only commands', () => {
    expect(classifyCommand('uname -a').risk).toBe('safe')
    expect(classifyCommand('ls -la /tmp').risk).toBe('safe')
    expect(classifyCommand('sed s/a/b/ file').risk).toBe('safe')
    expect(classifyCommand('cat x >/dev/null').risk).toBe('safe')
    expect(classifyCommand('cat x >&2').risk).toBe('safe')
    expect(classifyCommand('cat x >>&1').risk).toBe('safe')
    expect(classifyCommand('docker ps').risk).toBe('safe')
    expect(classifyCommand('docker images').risk).toBe('safe')
    expect(classifyCommand('kubectl get pods').risk).toBe('safe')
    expect(classifyCommand('git status').risk).toBe('safe')
    expect(classifyCommand('git log -1').risk).toBe('safe')
    expect(classifyCommand('systemctl status nginx').risk).toBe('safe')
    expect(classifyCommand('systemctl is-active nginx').risk).toBe('safe')
    expect(classifyCommand('crontab -l').risk).toBe('safe')
    expect(classifyCommand('crontab --list').risk).toBe('safe')
    expect(classifyCommand('find . -executable -name app').risk).toBe('safe')
    expect(classifyCommand('atq').risk).toBe('safe')
    expect(classifyCommand('ip addr show').risk).toBe('safe')
    expect(classifyCommand('pm2 list').risk).toBe('safe')
    expect(classifyCommand('docker compose ps').risk).toBe('safe')
    expect(classifyCommand('kubectl describe pod x').risk).toBe('safe')
    expect(classifyCommand('FOO=bar echo hi').risk).toBe('safe')
    expect(classifyCommand('timeout 5 ls').risk).toBe('safe')
    expect(classifyCommand('env --unset=FOO true').risk).toBe('safe')
    expect(classifyCommand('').risk).toBe('safe')
  })

  it('treats redirect-write as danger even for whitelist programs', () => {
    const result = classifyCommand('cat x > /tmp/out')
    expect(result.risk).toBe('danger')
    expect(result.readonly).toBe(false)
  })

  it('asks in production when the command is not read-only', () => {
    const result = classifyCommand('systemctl start nginx', { production: true })
    expect(result.risk).toBe('danger')
    expect(result.rules).toContain('production-not-readonly')
  })

  it('lets production read-only commands through', () => {
    expect(classifyCommand('ps aux', { production: true }).risk).toBe('safe')
  })

  it('lets extraDanger beat allow', () => {
    const extraDanger = [/safe-looking/]
    const allow = [/safe-looking/]
    const result = classifyCommand('safe-looking ls', { extraDanger, allow })
    expect(result.risk).toBe('danger')
    expect(result.rules[0]).toMatch(/^extra-danger/)
  })

  it('honors allow when extraDanger does not match', () => {
    expect(classifyCommand('rm -rf /tmp/x', { allow: [/^rm /] }).risk).toBe('safe')
  })

  it('treats readOnlyExtra programs as read-only', () => {
    const result = classifyCommand('myctl status', { production: true, readOnlyExtra: ['myctl'] })
    expect(result.risk).toBe('safe')
    expect(result.readonly).toBe(true)
  })

  it('does not treat identity wrappers alone as read-only', () => {
    const result = classifyCommand('sudo ls', { production: true })
    expect(result.risk).toBe('danger')
    expect(result.rules.some(r => r.startsWith('permission-identity'))).toBe(true)
  })

  it('peels timeout duration and env assignments before classifying', () => {
    expect(classifyCommand('timeout 10 sudo -u root rm -rf /tmp/x').rules.some(r => r.startsWith('destructive-fs'))).toBe(true)
    expect(programOf(tokenize('sudo --user=root rm /tmp/x')).program).toBe('rm')
  })
})

describe('classifyRemotePath', () => {
  it('marks write-sensitive destinations', () => {
    expect(classifyRemotePath('/etc/nginx/nginx.conf')).toBe('sensitive')
    expect(classifyRemotePath('/usr/bin/app')).toBe('sensitive')
    expect(classifyRemotePath('/etc/cron.d/job')).toBe('sensitive')
  })

  it('marks secret sources', () => {
    expect(classifyRemotePath('/root/.ssh/id_rsa')).toBe('secret')
    expect(classifyRemotePath('/home/a/.env')).toBe('secret')
    expect(classifyRemotePath('/tmp/cert.pem')).toBe('secret')
    expect(classifyRemotePath('/etc/shadow')).toBe('secret')
  })

  it('leaves ordinary paths alone', () => {
    expect(classifyRemotePath('/tmp/out.txt')).toBe('ok')
    expect(classifyRemotePath('/home/app/app.log')).toBe('ok')
  })

  it('marks secret names, key material, and cloud configs', () => {
    expect(classifyRemotePath('/home/a/.ssh/id_ed25519')).toBe('secret')
    expect(classifyRemotePath('/home/a/.ssh')).toBe('secret')
    expect(classifyRemotePath('/tmp/tls.key')).toBe('secret')
    expect(classifyRemotePath('/opt/app/credentials.json')).toBe('secret')
    expect(classifyRemotePath('/home/a/.env.local')).toBe('secret')
    expect(classifyRemotePath('/home/a/.aws/credentials')).toBe('secret')
    expect(classifyRemotePath('/home/a/.kube/config')).toBe('secret')
    expect(classifyRemotePath('/var/lib/db/shadow')).toBe('secret')
  })

  it('marks systemd units and crontab destinations as sensitive', () => {
    expect(classifyRemotePath('/etc')).toBe('sensitive')
    expect(classifyRemotePath('/boot/grub/grub.cfg')).toBe('sensitive')
    expect(classifyRemotePath('/lib/systemd/system/app.service')).toBe('sensitive')
    expect(classifyRemotePath('/usr/lib/systemd/system/app.timer')).toBe('sensitive')
    expect(classifyRemotePath('/run/systemd/system/app.service')).toBe('sensitive')
    expect(classifyRemotePath('/run/systemd/system/app.socket')).toBe('sensitive')
    expect(classifyRemotePath('/var/spool/cron/root')).toBe('sensitive')
    expect(classifyRemotePath('/home/a/crontab')).toBe('sensitive')
    expect(classifyRemotePath('/etc\\cron.d\\job')).toBe('sensitive')
  })
})

describe('normalizeCommand', () => {
  it('trims and collapses whitespace', () => {
    expect(normalizeCommand('  rm   -rf\t/tmp  ')).toBe('rm -rf /tmp')
  })
})
