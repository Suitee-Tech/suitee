# Diagrama de IAM, Permissões e Padrão Party/Role

Este diagrama detalha o ecossistema de Contas (Accounts), Usuários (Users), Autenticação (Credentials/Claims), Regras de Permissão de Acesso (Account Roles) e o Padrão de Projeto Party/Role (usado para mapear dinamicamente os papéis de negócio dos usuários no sistema).

```mermaid
classDiagram
    direction LR

    %% Relacionamentos Core IAM
    Account "1" *-- "0..1" User : Individual (Se Type=individual)
    Account "1" *-- "0..1" Company : Company (Se Type=company)
    
    Company "1" *-- "*" CompanyMember : Members
    CompanyMember "*" --> "1" Account : AccountID (Referência)
    CompanyMember ..> AccountRole : define permissões
    
    Credential "*" --> "1" Account : Autentica
    Credential ..> Claims : Gera Token (JWT)
    
    %% Padrão Party/Role
    Party "1" *-- "1" User : Representa
    Party "1" *-- "1..*" PartyRole : Assume papéis
    
    %% Classes de Autenticação e IAM
    class Account {
        +String ID
        +AccountType Type
        +Time CreatedAt
        +Time UpdatedAt
    }

    class User {
        +String ID
        +String FirstName
        +String LastName
        +String Email
        +String Role (Functional: broker, etc)
        +Boolean IsActive
        +String AvatarURL
    }

    class Company {
        +String ID
        +String Name
        +String TaxID
        +String OwnerID
        +Address Address
    }

    class CompanyMember {
        +String AccountID
        +String Profile (AccountRole)
    }

    class Credential {
        +String ID
        +String AccountID
        +String Email
        +String PasswordHash
        +Boolean IsVerified
        +String Role
    }

    class Claims {
        <<JWT Payload>>
        +String AccountID
        +String TenantID
        +String Role (Functional)
        +String Permission (Access Profile)
        +[]String PartnerTenantIDs
    }

    %% Padrão Party
    class Party {
        <<Contextual Entity>>
        +String ID
        +User User
        +[]PartyRole Roles
    }

    class AccountRole {
        <<Enumeration: Níveis de Acesso no Sistema>>
        owner
        admin
        manager
        member
        viewer
    }

    class PartyRole {
        <<Enumeration: Papéis de Negócio no CRM/Esteira>>
        seller
        buyer
        guarantor
        tenant
        owner
        other
    }
```
