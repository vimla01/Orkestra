import { useState, type FormEvent } from "react";
import { registerCluster } from "../api";
import { Modal } from "./Modal";

interface Props {
  onClose: () => void;
  onDone: (message: string) => void;
}

export function RegisterClusterDialog({ onClose, onDone }: Props) {
  const [name, setName] = useState("");
  const [kubeconfig, setKubeconfig] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      const cluster = await registerCluster(name.trim(), kubeconfig.trim());
      onDone(`Cluster ${cluster.name} registered`);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
      setBusy(false);
    }
  };

  return (
    <Modal
      title="Register cluster"
      onClose={onClose}
      footer={
        <>
          <button className="btn" onClick={onClose} type="button">
            Cancel
          </button>
          <button className="btn btn-primary" form="register-form" type="submit" disabled={busy || !name.trim() || !kubeconfig.trim()}>
            {busy ? "Connecting…" : "Register"}
          </button>
        </>
      }
    >
      <form id="register-form" className="form" onSubmit={submit}>
        <label className="field">
          <span className="field-label">Name</span>
          <input autoFocus value={name} onChange={(e) => setName(e.target.value)} placeholder="e.g. prod-eu-west" />
        </label>
        <label className="field">
          <span className="field-label">Kubeconfig path</span>
          <input
            className="mono"
            value={kubeconfig}
            onChange={(e) => setKubeconfig(e.target.value)}
            placeholder="/home/me/.kube/config"
          />
          <span className="field-hint">An absolute path on the machine running the control plane. Orkestra connects to the cluster to verify it.</span>
        </label>
        {error && <div className="form-error">{error}</div>}
      </form>
    </Modal>
  );
}
