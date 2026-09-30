import { useState, type FormEvent } from "react";
import { propagate, type PropagateResponse } from "../api";
import type { Cluster } from "../types";
import { Modal } from "./Modal";
import { StatusBadge, healthTone } from "./StatusBadge";

interface Props {
  clusters: Cluster[];
  onClose: () => void;
  onDone: () => void;
}

export function DeployDialog({ clusters, onClose, onDone }: Props) {
  const [manifest, setManifest] = useState("");
  const [selected, setSelected] = useState<string[]>([]);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [result, setResult] = useState<PropagateResponse | null>(null);

  const toggle = (name: string) =>
    setSelected((s) => (s.includes(name) ? s.filter((n) => n !== name) : [...s, name]));

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      // Keep the cluster order stable regardless of click order.
      const targets = clusters.map((c) => c.name).filter((n) => selected.includes(n));
      setResult(await propagate(manifest, targets));
      onDone();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  };

  if (result) {
    const failed = result.results.filter((r) => r.action === "Failed").length;
    return (
      <Modal
        title={`Propagated ${result.namespace}/${result.name}`}
        onClose={onClose}
        footer={
          <button className="btn btn-primary" onClick={onClose}>
            Done
          </button>
        }
      >
        <p className="muted">
          {failed === 0
            ? "Applied to every selected cluster. Rollout progress shows on the Deployments page."
            : `${failed} of ${result.results.length} clusters failed. The others were applied.`}
        </p>
        <ul className="result-list">
          {result.results.map((r) => (
            <li key={r.cluster}>
              <span className="mono">{r.cluster}</span>
              <StatusBadge tone={r.action === "Failed" ? "bad" : "ok"} label={r.action} />
              {r.error && <span className="result-error">{r.error}</span>}
            </li>
          ))}
        </ul>
      </Modal>
    );
  }

  return (
    <Modal
      title="New deployment"
      wide
      onClose={onClose}
      footer={
        <>
          <button className="btn" onClick={onClose} type="button">
            Cancel
          </button>
          <button
            className="btn btn-primary"
            form="deploy-form"
            type="submit"
            disabled={busy || !manifest.trim() || selected.length === 0}
          >
            {busy ? "Propagating…" : `Propagate to ${selected.length || ""} cluster${selected.length === 1 ? "" : "s"}`}
          </button>
        </>
      }
    >
      <form id="deploy-form" className="form deploy-form" onSubmit={submit}>
        <label className="field">
          <span className="field-label">Deployment manifest</span>
          <textarea
            className="mono"
            value={manifest}
            onChange={(e) => setManifest(e.target.value)}
            placeholder={"apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: my-app\nspec:\n  ..."}
            spellCheck={false}
            rows={16}
          />
          <span className="field-hint">YAML or JSON for an apps/v1 Deployment. It is created, or updated if it already exists.</span>
        </label>

        <fieldset className="field">
          <legend className="field-label">Target clusters</legend>
          {clusters.length === 0 ? (
            <p className="muted">Register a cluster first.</p>
          ) : (
            <div className="cluster-picks">
              {clusters.map((c) => (
                <label key={c.name} className={`pick ${selected.includes(c.name) ? "pick-on" : ""}`}>
                  <input type="checkbox" checked={selected.includes(c.name)} onChange={() => toggle(c.name)} />
                  <span className="pick-name">{c.name}</span>
                  <StatusBadge tone={healthTone(c.status)} label={c.status} />
                </label>
              ))}
            </div>
          )}
        </fieldset>
        {error && <div className="form-error">{error}</div>}
      </form>
    </Modal>
  );
}
