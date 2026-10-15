#!/usr/bin/env python3
"""Root-only Linux acceptance against an isolated real old dockerd.

Requires official Docker static binaries, unshare, iproute2 and an iperf
image archive. Uses a private network namespace as old bridge initialization
can delete docker0 even with --bridge=none. Never shares host daemon data.
"""
import argparse,json,os,pathlib,re,shutil,socket,subprocess,time,urllib.request

a=argparse.ArgumentParser()
a.add_argument('--docker-bin-dir',required=True)
a.add_argument('--workdir',required=True)
a.add_argument('--agent-bin',required=True)
a.add_argument('--previous-agent-bin',help='Optional old binary: must reproduce the API 1.44 rejection')
a.add_argument('--image-archive',required=True)
a.add_argument('--test-image',default='luna-ebpf-iperf-test:alpine3.21.3')
a.add_argument('--backend',default='auto',choices=['auto','socket-trace'])
args=a.parse_args()
root=pathlib.Path(args.workdir).resolve();root.mkdir(parents=True,exist_ok=True)
assert not any(root.iterdir()),'workdir must be new and empty'
assert shutil.disk_usage(root).free>=128*1024*1024,'workdir requires at least 128 MiB free space'
bins=pathlib.Path(args.docker_bin_dir).resolve();sock=root/'docker.sock'
assert len(str(sock))<100
env=dict(os.environ,PATH=str(bins)+':'+os.environ['PATH'])
env.pop('DOCKER_API_VERSION',None);env.pop('DOCKER_HOST',None)
docker=[str(bins/'docker'),'--host','unix://'+str(sock)]
daemon=None;agent=None;names=[];logs=[]
host_netns=os.readlink('/proc/self/ns/net')
bridge_index=pathlib.Path('/sys/class/net/docker0/ifindex')
host_bridge=bridge_index.read_text() if bridge_index.exists() else None
def run(*argv):return subprocess.check_output(argv,text=True,env=env).strip()
def start_daemon():
 global daemon
 log=open(root/'dockerd.log','a');logs.append(log)
 daemon=subprocess.Popen(['unshare','--net','--','sh','-ec','ip link set lo up; exec "$@"','luna-test-dockerd',str(bins/'dockerd'),'--config-file='+str(root/'daemon.json'),'--host=unix://'+str(sock),'--data-root='+str(root/'data'),'--exec-root='+str(root/'exec'),'--pidfile='+str(root/'docker.pid'),'--bridge=none','--iptables=false','--ip-forward=false','--ip-masq=false','--storage-driver=vfs'],env=env,stdout=log,stderr=subprocess.STDOUT)
 for _ in range(60):
  if daemon.poll() is not None:raise RuntimeError('dockerd failed; see dockerd.log')
  try:
   run(*docker,'info')
   daemon_netns=os.readlink('/proc/'+str(daemon.pid)+'/ns/net')
   assert daemon_netns!=host_netns,'daemon must not share the host network namespace'
   (root/'network-isolation.json').write_text(json.dumps({'host':host_netns,'daemon':daemon_netns,'host_docker0_ifindex':host_bridge}))
   return
  except subprocess.CalledProcessError:time.sleep(1)
 raise RuntimeError('dockerd timeout')
def stop(proc):
 if proc is not None and proc.poll() is None:
  proc.terminate()
  try:proc.wait(timeout=30)
  except subprocess.TimeoutExpired:proc.kill();proc.wait(timeout=10)
def scrape():
 with urllib.request.urlopen('http://127.0.0.1:19834/metrics',timeout=5) as r:return r.read().decode()
def wait_for(check):
 last=None
 for _ in range(45):
  try:
   last=scrape()
   if check(last):return last
  except Exception:pass
  time.sleep(1)
 raise AssertionError('metrics condition timed out')
def counters(text,cid):
 out={}
 for direction in ['rx','tx']:
  for unit in ['bytes','packets']:
   name=f'luna_container_{direction}_{unit}_total'
   m=re.search(name+r'\{container_id="'+cid+r'",container_name="[^"]*"\} ([\d.eE+\-]+)',text)
   assert m,(name,cid)
   out[direction+'_'+unit]=int(float(m[1]))
 return out
try:
 # Only the native exporter listens in the host namespace.
 for port in [19834]:
  with socket.socket() as s:
   s.setsockopt(socket.SOL_SOCKET,socket.SO_REUSEADDR,1)
   s.bind(('127.0.0.1',port))
 (root/'daemon.json').write_text('{}')
 start_daemon()
 version=json.loads(run(*docker,'version','--format','{{json .Server}}'))
 (root/'version.json').write_text(json.dumps(version,indent=2))
 if args.previous_agent_bin:
  old=subprocess.run([str(pathlib.Path(args.previous_agent_bin).resolve()),'--docker-socket',str(sock),'--check'],capture_output=True,text=True,timeout=20)
  (root/'previous-version-check.log').write_text(old.stdout+old.stderr)
  assert old.returncode!=0 and 'minimum supported API version is 1.44' in old.stderr,old
 run(*docker,'load','-i',str(pathlib.Path(args.image_archive).resolve()))
 ids={}
 for role,command in [('server',['iperf3','-s','-p','15434']),('client',['sleep','3600']),('idle',['sleep','3600'])]:
  name='luna-api-'+role;names.append(name)
  ids[role]=run(*docker,'run','-d','--name',name,'--network','host',args.test_image,*command)
 log=open(root/'agent.log','w');logs.append(log)
 agent=subprocess.Popen([str(pathlib.Path(args.agent_bin).resolve()),'--docker-socket',str(sock),'--listen-address','127.0.0.1:19834','--collector-backend',args.backend,'--metrics-poll-interval','1s','--docker-reconcile-interval','30s','--container-stale-grace-period','2s'],stdout=log,stderr=subprocess.STDOUT)
 wait_for(lambda t:'luna_exporter_up 1' in t and 'container_id="'+ids['client']+'"' in t)
 results=[]
 for label,extra in [('tcp',[]),('udp-reverse',['-u','-R'])]:
  time.sleep(2);before=scrape();(root/(label+'-before.metrics')).write_text(before)
  payload=json.loads(run(*docker,'exec',names[1],'iperf3','-c','127.0.0.1','-p','15434','-t','2','-b','16M','-J',*extra))
  (root/(label+'-iperf.json')).write_text(json.dumps(payload))
  assert 'error' not in payload,payload
  time.sleep(3);after=scrape();(root/(label+'-after.metrics')).write_text(after)
  tx,rx=('server','client') if extra else ('client','server')
  changes={role:{k:v-counters(before,cid)[k] for k,v in counters(after,cid).items()} for role,cid in ids.items()}
  sent=payload['end']['sum_sent']['bytes']
  for role,direction in [(tx,'tx'),(rx,'rx')]:
   assert sent*.9<=changes[role][direction+'_bytes']<=sent*1.25,changes
   assert changes[role][direction+'_packets']>0,changes
  assert all(v==0 for v in changes['idle'].values()),changes
  results.append({'test':label,'deltas':changes});print('PASS',label,changes,flush=True)
 run(*docker,'rename',names[1],names[1]+'-renamed');names[1]+='-renamed'
 wait_for(lambda t:'container_name="'+names[1]+'"' in t)
 run(*docker,'restart',names[1]);wait_for(lambda t:'luna_exporter_up 1' in t)
 run(*docker,'rm','-f',names[1]);names.pop(1)
 wait_for(lambda t:'container_id="'+ids['client']+'"' not in t)
 for name in names:run(*docker,'rm','-f',name)
 names.clear();stop(daemon);start_daemon()
 final=wait_for(lambda t:'luna_exporter_up 1' in t and 'luna_exporter_docker_connected 1' in t)
 (root/'recovered.metrics').write_text(final)
 (root/'results.json').write_text(json.dumps(results,indent=2))
 print('PASS discovery, events rename, restart, retirement, daemon disconnect/recovery',flush=True)
finally:
 for name in names:subprocess.run([*docker,'rm','-f',name],env=env,stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
 stop(agent);stop(daemon)
 for log in logs:log.close()
 current_bridge=bridge_index.read_text() if bridge_index.exists() else None
 assert current_bridge==host_bridge,'host docker0 changed during isolated validation'
