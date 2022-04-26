import React, { ChangeEvent, FC, useEffect } from 'react';
import { Input, InputGroup, InputGroupAddon, InputGroupText } from 'reactstrap';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { faSearch } from '@fortawesome/free-solid-svg-icons';

export interface SearchBarProps {
  handleChange: (e: string) => void;
  placeholder: string;
  defaultValue: string;
}

const SearchBar: FC<SearchBarProps> = ({ handleChange, placeholder, defaultValue }) => {
  let filterTimeout: NodeJS.Timeout;

  const handleSearchChange = (e: ChangeEvent<HTMLTextAreaElement | HTMLInputElement>) => {
    clearTimeout(filterTimeout);
    filterTimeout = setTimeout(() => {
      handleChange(e.target.value);
    }, 300);
  };

  useEffect(() => {
    handleChange(defaultValue);
  }, [defaultValue, handleChange]);

  return (
    <InputGroup>
      <InputGroupAddon addonType="prepend">
        <InputGroupText>{<FontAwesomeIcon icon={faSearch} />}</InputGroupText>
      </InputGroupAddon>
      <Input autoFocus onChange={handleSearchChange} placeholder={placeholder} defaultValue={defaultValue} />
    </InputGroup>
  );
};

export default SearchBar;
