# Diagrama de Entidades e Relacionamentos (UML)

Este diagrama ilustra as principais entidades da aplicação, suas propriedades, métodos e os relacionamentos entre elas.
Você pode visualizar este diagrama usando uma extensão de visualização de Markdown/Mermaid no seu editor, ou colando o código abaixo no [Mermaid Live Editor](https://mermaid.live/).

```mermaid
classDiagram
    %% Relacionamentos
    Account "1" -- "*" Lead : isola dados (TenantID)
    Account "1" -- "*" Property : isola dados (TenantID)
    Account "1" -- "*" Broker : isola dados (TenantID)
    
    Broker "1" -- "*" Lead : gerencia (AssignedBrokerID)
    Broker "1..*" -- "*" Property : capta (CapturingBrokers)
    
    Lead "1..*" -- "*" Property : interessa / possui
    
    Property "1" -- "*" Deal : é objeto de
    Lead "1..*" -- "*" Deal : participa (Owner/Customer)
    Broker "1..*" -- "*" Deal : intermedia (BrokerParticipations)
    
    Deal "1" -- "1" Contract : gera
    Lead "1..*" -- "*" Contract : assina (Signatories)
    
    Contract "1" -- "*" Transaction : gera (cobranças/repasses)

    %% Classes e Entidades
    class Account {
        +String ID
        +String Type
        +Object Individual
        +Object Company
        +login()
        +getTenantID()
        +updateProfile()
    }

    class Lead {
        +String ID
        +String Name
        +String Email
        +String Phone
        +String Status
        +String Source
        +Boolean IsClient
        +String Role
        +String AssignedBrokerID
        +convertToClient()
        +assignBroker(brokerID)
        +updateStatus(newStatus)
    }

    class Broker {
        +String ID
        +String Name
        +String Role
        +String Email
        +Boolean Active
        +Int MaxActiveLeads
        +assignLead(leadID)
        +participateInDeal(dealID)
    }

    class Property {
        +String ID
        +String Title
        +String Status
        +String PropertyType
        +String ListingType
        +Float Pricing
        +Object Address
        +updateStatus(newStatus)
        +addOwner(leadID)
        +addInterestedLead(leadID)
    }

    class Deal {
        +String ID
        +String Type
        +String Status
        +Float Value
        +Float Commission
        +String PropertyID
        +String OwnerID
        +String CustomerID
        +String BrokerID
        +addProposal(proposal)
        +approveDueDiligence()
        +generateContract()
    }

    class Contract {
        +String ID
        +String Status
        +String Type
        +Float Value
        +Date StartDate
        +Date EndDate
        +Float CommissionPct
        +String DealID
        +String PropertyID
        +sign(signatoryID)
        +renew(newEndDate)
        +generateTransactions()
    }

    class Transaction {
        +String ID
        +String Type
        +Float Value
        +String Status
        +Date Date
        +String PaymentMethod
        +String ContractID
        +processPayment()
        +cancel()
        +refund()
    }
```
