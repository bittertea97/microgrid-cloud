import React from 'react'
import { Typography } from 'antd'

interface PlaceholderCardProps {
  title: string
  description: string
}

export const PlaceholderCard: React.FC<PlaceholderCardProps> = ({ title, description }) => {
  return (
    <div className="placeholder-card">
      <Typography.Title level={3} style={{ marginTop: 0 }}>
        {title}
      </Typography.Title>
      <Typography.Paragraph style={{ marginBottom: 0 }}>
        {description}
      </Typography.Paragraph>
    </div>
  )
}
