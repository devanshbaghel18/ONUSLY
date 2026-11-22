import { LockKeyhole, LockKeyholeOpen } from "lucide-react";

// The ring, spindle, face and hinged retaining bar occupy separate depth planes.
export default function LockMechanism({ released = false, reviewing = false }) {
  const Icon = released ? LockKeyholeOpen : LockKeyhole;
  return (
    <div className="mechanism-stage" aria-hidden="true">
      <div className={'mechanism' + (released ? ' released' : '') + (reviewing ? ' reviewing' : '')}>
        {[0, 1, 2, 3, 4, 5].map(layer => <i key={layer} className="drum-layer" style={{ transform: 'translateZ(' + layer * 4 + 'px)' }} />)}
        <div className="drum-face"><div className="dial"><Icon size={34} strokeWidth={2} /></div></div>
        <div className="retainer"><i /><span /><i /></div>
      </div>
    </div>
  );
}
