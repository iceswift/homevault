from datetime import datetime, timedelta, timezone
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import subprocess
import sys
import tarfile
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location('workflow', Path(__file__).with_name('planned-workflow.py'))
m = importlib.util.module_from_spec(spec)
spec.loader.exec_module(m)


class PolicyTest(unittest.TestCase):
    def test_capture_age_not_upload_age_controls_health(self):
        at = datetime(2026, 10, 25, tzinfo=timezone.utc)
        w = {'enabled': True, 'max_age_hours': 36}
        state = {'last_success': at.isoformat(), 'captured_at': (at-timedelta(hours=40)).isoformat(), 'latest': {'result':'skipped'}}
        self.assertEqual(m.health(state,w,at), 'overdue')
        self.assertEqual(m.health(state,dict(w,enabled=False),at), 'not-activated')
        state['captured_at']=(at+timedelta(hours=1)).isoformat()
        with self.assertRaises(m.Stop):m.health(state,w,at)

    def test_failure_before_first_upload_and_delivery_transitions(self):
        w={'enabled':True,'max_age_hours':36}
        self.assertEqual(m.health({'latest':{'result':'failed'}},w),'failed')
        self.assertEqual(m.alert_event('failed',None),'failed')
        self.assertIsNone(m.alert_event('failed','failed'))
        self.assertEqual(m.alert_event('healthy','failed'),'recovered')
        self.assertIsNone(m.alert_event('healthy','recovered'))
        self.assertEqual(m.alert_event('overdue','recovered'),'stale')
        self.assertIsNone(m.alert_event('running','failed'))


@unittest.skipUnless(os.name=='posix','Linux coordinator/permissions/locks')
class CoordinatorTest(unittest.TestCase):
    def setUp(self):
        self.temp=tempfile.TemporaryDirectory();self.root=Path(self.temp.name);self.root.chmod(0o700)
        self.hook=self.root/'hook';self.hook.write_text('fixture-only');self.hook.chmod(0o600)
        self.base=self.root/'base.json';self.base.write_text(json.dumps({'version':1,'repository':str(self.root/'repo'),'state_dir':str(self.root/'go-state'),'password_file':str(self.root/'key'),'discord_webhook_file':str(self.hook)}))
        self.source=self.root/'source';self.source.write_text('fixture');self.source.chmod(0o600)
        self.calls=[]
        self.w={'name':'static','project':'static-project','kind':'files','enabled':False,'max_age_hours':36,'files':[{'source':str(self.source),'target':'deploy/file'}],'trees':[]}
        self.plan={'version':1,'root':str(self.root/'suite'),'base_config':str(self.base),'homevault_binary':'/fixture/homevault','notifications_enabled':True,'workflows':[self.w]}
    def tearDown(self):self.temp.cleanup()
    def suite(self,runner=None):
        p=self.root/'plan.json';p.write_text(json.dumps(self.plan))
        return m.Workflow(p,runner or self.fake)
    def fake(self,args,**kw):
        self.calls.append(args)
        return b''
    def test_disabled_and_missing_window_never_call_capture(self):
        suite=self.suite();r=suite.run(suite.workflows['static'])
        self.assertEqual(r['result'],'not-activated');self.assertFalse(self.calls)
        w=dict(self.w,enabled=True,kind='json',window=None)
        self.assertEqual(suite.run(w)['result'],'outside-maintenance-window');self.assertFalse(self.calls)
    def test_staging_failure_preserves_previous_success_and_alert_can_recover(self):
        self.w['enabled']=True;suite=self.suite();w=suite.workflows['static']
        original=m.stamp();state={'version':1,'binding':suite.binding(w),'last_success':original,'captured_at':original,'latest':{'result':'success'},'history':[]}
        suite.persist(w,state)
        self.source.unlink()
        with self.assertRaises(m.Stop):suite.run(w)
        after=suite.status(w);self.assertEqual(after['last_success'],original);self.assertEqual(after['latest']['result'],'failed')
        self.assertEqual(suite.monitor(w)['notification'],'sent')
        self.assertEqual(suite.monitor(w)['notification'],'deduplicated-or-not-needed')
        self.assertEqual(len(self.calls),1)
        after['latest']={'result':'success'};suite.persist(w,after)
        self.assertEqual(suite.monitor(w)['notification'],'sent')
        self.assertEqual(self.calls[-1][-2:],[ 'recovered','--confirm'])
    def test_failed_delivery_does_not_mark_deduplicated_or_destroy_success(self):
        self.w['enabled']=True
        def fail(*a,**kw):raise m.Stop('fixture-delivery-failure')
        suite=self.suite(fail);w=suite.workflows['static']
        at=m.stamp();suite.persist(w,{'version':1,'binding':suite.binding(w),'last_success':at,'captured_at':at,'latest':{'result':'failed'},'history':[]})
        with self.assertRaises(m.Stop):suite.monitor(w)
        self.assertFalse((suite.state_dir/'static-alert.json').exists())
        self.assertEqual(suite.status(w)['last_success'],at)
    def test_verified_job_success_is_separate_from_delivery(self):
        self.w['enabled']=True
        def fake(args,**kw):
            self.calls.append(args)
            if 'status' in args:
                at=m.stamp()
                return json.dumps({'last_success':at,'latest':{'id':'fixture-job','status':'success','backup_passed':True,'check_passed':True,'started':at}}).encode()
            if 'notify-event' in args:raise m.Stop('delivery-failed')
            return b''
        suite=self.suite(fake);w=suite.workflows['static']
        self.assertEqual(suite.run(w)['result'],'success')
        before=suite.status(w)['last_success']
        self.assertFalse(any('notify-event' in c for c in self.calls))
        # No routine success message; simulate a prior failed alert to test recovery delivery.
        m.save(suite.state_dir/'static-alert.json',{'binding':suite.binding(w),'event':'failed'})
        with self.assertRaises(m.Stop):suite.monitor(w)
        self.assertEqual(suite.status(w)['last_success'],before)
        stage=next(suite.stages.glob('static-*/bundle'))
        self.assertTrue((stage/'capture-info.json').is_file())
    def test_plan_or_repository_change_rejects_old_green_state(self):
        self.w['enabled']=True;suite=self.suite();w=suite.workflows['static']
        suite.persist(w,{'version':1,'binding':'wrong-repo','latest':{'result':'success'},'history':[]})
        with self.assertRaises(m.Stop):suite.monitor(w,True)
    def test_lock_collision_and_output_limit(self):
        lock=self.root/'lock'
        with m.locked(lock):
            with self.assertRaises(m.Stop):
                with m.locked(lock):pass
        with self.assertRaises(m.Stop):m.command([sys.executable,'-c',"print('x'*100000)"],output=True,limit=1000)
    def test_sealed_bundle_rejects_bad_hash_and_stale_source(self):
        root=self.root/'sealed';root.mkdir()
        folder=root/(m.now().strftime('%Y%m%dT%H%M%SZ')+'-123');folder.mkdir()
        names=['database.dump','uploads.tar.gz','images.json','manifest.txt']
        (folder/'database.dump').write_bytes(b'fixture')
        with tarfile.open(folder/'uploads.tar.gz','w:gz'):pass
        (folder/'images.json').write_text('[]');(folder/'manifest.txt').write_text('fixture')
        (folder/'.saan-scheduled-complete').write_text('format=1\n')
        (folder/'SHA256SUMS').write_text(''.join(m.digest(folder/n)+'  '+n+'\n' for n in names))
        w={'source_root':str(root),'max_age_hours':36,'image_pins':[],'deployment_files':[]}
        suite=self.suite();self.assertEqual(suite.sealed_source(w)[0],folder)
        (folder/'database.dump').write_bytes(b'corrupted')
        with self.assertRaises(m.Stop):suite.sealed_source(w)
    def test_no_duplicate_bundle_upload_or_freshness_advance(self):
        self.w.clear();self.w.update(name='sealed',project='sealed-project',kind='sealed',enabled=True,max_age_hours=36,source_root=str(self.root),source_lock=str(self.root/'source.lock'),deployment_files=[],image_pins=[])
        suite=self.suite();w=suite.workflows['sealed'];at=m.stamp()
        suite.preflight=lambda _:None;suite.sealed_source=lambda _:(self.root,at,'same-source')
        suite.persist(w,{'version':1,'binding':suite.binding(w),'last_success':at,'captured_at':at,'source_id':'same-source','latest':{'result':'success'},'history':[]})
        result=suite.run(w);self.assertEqual(result['result'],'already-uploaded-no-freshness-advance');self.assertFalse(self.calls)
        self.assertEqual(suite.status(w)['last_success'],at)
    def test_json_copy_failure_resumes_exact_container_before_return(self):
        import io
        running=True;calls=[]
        cid='a'*64;image='sha256:'+'b'*64
        def backend(args,**kw):
            nonlocal running
            calls.append(args[:2])
            if args[:2]==['docker','inspect']:
                return json.dumps({'id':cid,'image':image,'running':running,'health':'healthy','mounts':[{'Type':'bind','Source':'/fixture/data','Destination':'/app/data'}]}).encode()
            if args[:2]==['docker','stop']:running=False
            if args[:2]==['docker','start']:running=True
            if args[:2]==['docker','cp']:
                out=io.BytesIO()
                with tarfile.open(fileobj=out,mode='w') as tar:
                    entry=tarfile.TarInfo('one.json');entry.size=3;tar.addfile(entry,io.BytesIO(b'bad'))
                return out.getvalue()
            return b''
        suite=self.suite(backend)
        w={'name':'gold','container_id':cid,'image_id':image,'mount_source':'/fixture/data','mount_target':'/app/data','files':['one.json'],'window':None,'max_pause_seconds':20}
        class Guardian:
            def __init__(self,args,**kw):
                guard=Path(args[-1]);nonce=json.loads(guard.read_text())['nonce'];m.save(str(guard)+'.ready',{'nonce':nonce})
            def poll(self):return None
        with patch.object(m.subprocess,'Popen',Guardian):
            with self.assertRaises(json.JSONDecodeError):suite.json_capture(w,suite.stages/'captured')
        self.assertTrue(running)
        self.assertIn(['docker','start'],calls)
        self.assertFalse((suite.guards/'gold.json').exists())
    def test_reviewed_window_wraps_midnight(self):
        at=datetime(2026,10,25,0,4,tzinfo=timezone.utc)
        self.assertTrue(m.window_open({'timezone':'UTC','start':'23:55','minutes':15},at))
        self.assertFalse(m.window_open({'timezone':'UTC','start':'23:55','minutes':15},at+timedelta(minutes=20)))


if __name__=='__main__':unittest.main()
