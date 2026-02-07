import React from 'react';
import { formatTimestamp } from '../utils/format';
import './ChatMessage.css';

interface Message {
  id: string;
  text: string;
  sender: 'user' | 'agent';
  timestamp: Date;
}

interface ChatMessageProps {
  message: Message;
}

export const ChatMessage: React.FC<ChatMessageProps> = ({ message }) => {
  return (
    <div className={`chat-message ${message.sender}`}>
      <div className="message-bubble">
        <p>{message.text}</p>
        <span className="message-time">{formatTimestamp(message.timestamp)}</span>
      </div>
    </div>
  );
};
