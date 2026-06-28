import type { Group } from '../types'

interface Props {
  group: Group
}

export default function GroupCard({ group }: Props) {
  const sorted = [...group.competitors].sort((a, b) => b.points - a.points || b.won - a.won)

  return (
    <div className="rounded p-4 space-y-3 border" style={{ backgroundColor: 'var(--color-surface-card)', borderColor: 'var(--color-border)' }}>
      <h3 className="text-xs uppercase tracking-widest font-bold" style={{ color: 'var(--color-brand)' }}>
        {group.name}
      </h3>
      <table className="w-full text-sm">
        <thead>
          <tr className="text-xs uppercase tracking-widest" style={{ color: '#777' }}>
            <th className="text-left pb-2 font-normal">Player</th>
            <th className="text-right pb-2 font-normal w-6">W</th>
            <th className="text-right pb-2 font-normal w-6">L</th>
            <th className="text-right pb-2 font-normal w-8">Pts</th>
          </tr>
        </thead>
        <tbody>
          {sorted.map((c, i) => (
            <tr key={c.id} className="border-t" style={{ borderColor: 'var(--color-border-subtle)', color: i === 0 ? 'var(--color-brand)' : i === 1 ? 'var(--color-wax-red)' : '#f0f0f0' }}>
              <td className="py-1.5 pr-2 truncate max-w-[120px]">{c.name}</td>
              <td className="py-1.5 text-right tabular-nums">{c.won}</td>
              <td className="py-1.5 text-right tabular-nums" style={{ color: '#888' }}>{c.lost}</td>
              <td className="py-1.5 text-right tabular-nums font-semibold">{c.points}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}
