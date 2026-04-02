import { LocalTerminalTab as SharedLocalTerminalTab } from '@skyhook-io/k8s-ui'

interface LocalTerminalTabProps {
  isActive?: boolean
  initialCommand?: string
}

export function LocalTerminalTab({ isActive, initialCommand }: LocalTerminalTabProps) {
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'

  const createSession = () =>
    Promise.resolve({
      wsUrl: `${protocol}//${window.location.host}/api/local-terminal`,
    })

  return (
    <SharedLocalTerminalTab
      isActive={isActive}
      createSession={createSession}
      initialCommand={initialCommand}
    />
  )
}
