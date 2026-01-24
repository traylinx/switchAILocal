import React from 'react';
import { render, screen, fireEvent } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { StringArrayInput } from './StringArrayInput';

describe('StringArrayInput', () => {
    const mockOnChange = jest.fn();

    beforeEach(() => {
        mockOnChange.mockClear();
    });

    test('renders with label', () => {
        render(<StringArrayInput label="Excluded models" onChange={mockOnChange} />);
        expect(screen.getByText('Excluded models')).toBeInTheDocument();
    });

    test('renders initial values as tags', () => {
        const values = ['gpt-3.5', 'claude-2'];
        render(<StringArrayInput label="Excluded models" values={values} onChange={mockOnChange} />);
        
        expect(screen.getByText('gpt-3.5')).toBeInTheDocument();
        expect(screen.getByText('claude-2')).toBeInTheDocument();
    });

    test('adds value on Enter', async () => {
        const user = userEvent.setup();
        render(<StringArrayInput label="Excluded models" onChange={mockOnChange} />);
        
        const input = screen.getByRole('textbox');
        await user.type(input, 'llama-2{enter}');

        expect(mockOnChange).toHaveBeenCalledWith(['llama-2']);
        expect(input).toHaveValue('');
    });

    test('adds value on blur', async () => {
        const user = userEvent.setup();
        render(<StringArrayInput label="Excluded models" onChange={mockOnChange} />);
        
        const input = screen.getByRole('textbox');
        await user.type(input, 'mistral');
        fireEvent.blur(input);

        expect(mockOnChange).toHaveBeenCalledWith(['mistral']);
    });

    test('does not add empty or duplicate values', async () => {
        const user = userEvent.setup();
        const values = ['existing'];
        render(<StringArrayInput label="Excluded models" values={values} onChange={mockOnChange} />);
        
        const input = screen.getByRole('textbox');
        
        // Try empty
        await user.type(input, '{enter}');
        expect(mockOnChange).not.toHaveBeenCalled();

        // Try duplicate
        await user.type(input, 'existing{enter}');
        expect(mockOnChange).not.toHaveBeenCalled();
    });

    test('removes value when X clicked', async () => {
        const user = userEvent.setup();
        const values = ['remove-me'];
        render(<StringArrayInput label="Excluded models" values={values} onChange={mockOnChange} />);
        
        const removeButton = screen.getByRole('button');
        await user.click(removeButton);

        expect(mockOnChange).toHaveBeenCalledWith([]);
    });
});
