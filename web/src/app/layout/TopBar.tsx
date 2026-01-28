import React from 'react'
import { Layout, Space, Typography } from 'antd'

const { Header } = Layout

export const TopBar: React.FC = () => {
  return (
    <Header
      style={{
        background: '#ffffff',
        borderBottom: '1px solid #e6ebf2',
        padding: '0 24px'
      }}
    >
      <Space style={{ height: '100%', display: 'flex', alignItems: 'center' }}>
        <Typography.Text style={{ fontWeight: 600, color: '#0b1f33' }}>
          Operations Console
        </Typography.Text>
        {/* BEGIN AGENT-2 TOPBAR CONTROLS */}
        {/* END AGENT-2 TOPBAR CONTROLS */}
      </Space>
    </Header>
  )
}
