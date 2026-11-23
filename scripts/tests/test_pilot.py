import importlib.util
import json
import pathlib
import tempfile
import unittest

spec = importlib.util.spec_from_file_location("pilot", pathlib.Path(__file__).parents[1] / "pilot.py")
pilot = importlib.util.module_from_spec(spec)
spec.loader.exec_module(pilot)


def write_session(out, pid, n, phase):
    folder = out / pid
    folder.mkdir(parents=True, exist_ok=True)
    record = {"status": {"snapshot": {"state": {"phase": phase}}}}
    (folder / f"session-{n}.json").write_text(json.dumps(record))


def write_log(out, pid, rows):
    lines = ["timestamp,session,kind,step,words"] + rows
    (out / pid / "log.csv").write_text("\n".join(lines) + "\n")


class Summary(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.out = pathlib.Path(self.tmp.name)

    def tearDown(self):
        self.tmp.cleanup()

    def test_ready_when_enough_reach_production_unaided(self):
        for i in range(5):
            write_session(self.out, f"P0{i}", 1, "planned")
            write_session(self.out, f"P0{i}", 2, "production_verified")
        write_log(self.out, "P00", ["10:00,1,stop,verify,checks inconclusive"])
        s = pilot.summarize(self.out)
        self.assertEqual(s["criteria"]["share_unaided_to_production"], 1.0)
        self.assertTrue(s["criteria"]["ready_on_measured_criteria"])
        self.assertEqual(s["participants"][0]["phase"], "production_verified")

    def test_session_order_is_numeric(self):
        write_session(self.out, "P01", 2, "draft")
        write_session(self.out, "P01", 10, "released")
        self.assertEqual(pilot.summarize(self.out)["participants"][0]["phase"], "released")

    def test_interventions_breaches_and_repeated_obstacles_fail_readiness(self):
        for pid in ("P01", "P02", "P03", "P04", "P05"):
            write_session(self.out, pid, 1, "production_verified")
        write_log(self.out, "P01", ["10:00,1,intervention,doctor,installed podman for them"])
        write_log(self.out, "P02", ["10:00,1,stop,Doctor,no container runtime"])
        write_log(self.out, "P03", ["10:00,1,stop,doctor,no container runtime", "11:00,1,fabricated_reviewer,review,typed a friend's name"])
        c = pilot.summarize(self.out)["criteria"]
        self.assertEqual(c["share_unaided_to_production"], 0.8)
        self.assertFalse(c["no_integrity_breach"])
        self.assertEqual(c["repeated_obstacles"], ["doctor"])
        self.assertFalse(c["ready_on_measured_criteria"])

    def test_unknown_log_kind_is_rejected(self):
        write_session(self.out, "P01", 1, "draft")
        write_log(self.out, "P01", ["10:00,1,complaint,x,y"])
        with self.assertRaises(ValueError):
            pilot.summarize(self.out)

    def test_participant_ids_are_slugs(self):
        with self.assertRaises(ValueError):
            pilot.collect(self.out, "../x", 1, self.out, "agentarch")


if __name__ == "__main__":
    unittest.main()
